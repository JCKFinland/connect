package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type leaseVerificationProvider struct {
	calls        int
	details      payouts.VerifiedTransferDetails
	beforeReturn func()
}

func (p *leaseVerificationProvider) LookupTransferDetails(
	_ context.Context,
	_ payouts.TransferDetailsLookup,
) (payouts.VerifiedTransferDetails, error) {
	p.calls++
	if p.beforeReturn != nil {
		p.beforeReturn()
	}
	return p.details, nil
}

func TestLeaseFencedVerificationDatabaseBoundaries(t *testing.T) {
	fixture := newSubmissionFixture(t)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 15*time.Second,
		)
		defer cancel()
		if _, err := fixture.db.Exec(ctx, `
            DELETE FROM driver_payout_recovery_leases
            WHERE payout_id=$1::uuid
        `, fixture.payoutID); err != nil {
			t.Errorf("lease cleanup: %v", err)
		}
	})

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			fixture.ctx, fixture.db, fixture.payoutID,
		)
	if err != nil || !claimed || execution == nil {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	lease, acquired, err :=
		postgresrepo.AcquireDriverPayoutRecoveryLease(
			fixture.ctx, fixture.db, fixture.payoutID,
			"verification-integration-owner",
			90*time.Second, 5*time.Minute,
		)
	if err != nil || !acquired || lease == nil {
		t.Fatalf("lease: acquired=%v err=%v", acquired, err)
	}

	expected, err :=
		postgresrepo.ResolveDriverPayoutRecoveryTransferRequest(
			fixture.ctx, fixture.db, fixture.payoutID,
		)
	if err != nil || expected == nil {
		t.Fatalf("resolve identity: %v", err)
	}

	details := successfulVerifiedReconciliationDetails(*expected)

	verify := func(
		provider *leaseVerificationProvider,
		token string,
	) (*payouts.VerifiedTransferDetails, error) {
		t.Helper()
		service, err := NewLeaseFencedVerificationService(
			fixture.db, provider,
		)
		if err != nil {
			t.Fatal(err)
		}
		return service.VerifyExistingTransfer(
			fixture.ctx, fixture.payoutID,
			token, 5*time.Second,
		)
	}

	t.Run("valid_owner", func(t *testing.T) {
		provider := &leaseVerificationProvider{details: details}
		result, err := verify(provider, lease.Token)
		if err != nil || result == nil || provider.calls != 1 {
			t.Fatalf(
				"valid verification: result=%+v calls=%d err=%v",
				result, provider.calls, err,
			)
		}
	})

	t.Run("invalid_token", func(t *testing.T) {
		provider := &leaseVerificationProvider{details: details}
		result, err := verify(provider, uuid.NewString())
		if result != nil ||
			!errors.Is(err, ErrLeaseFencedVerificationDenied) ||
			provider.calls != 0 {
			t.Fatalf(
				"invalid token accepted: result=%+v calls=%d err=%v",
				result, provider.calls, err,
			)
		}
	})

	t.Run("replaced_during_lookup", func(t *testing.T) {
		provider := &leaseVerificationProvider{details: details}
		provider.beforeReturn = func() {
			tag, err := fixture.db.Exec(fixture.ctx, `
                UPDATE driver_payout_recovery_leases
                SET lease_token=$2::uuid,
                    lease_owner='replacement-owner',
                    updated_at=clock_timestamp()
                WHERE payout_id=$1::uuid
                  AND lease_token=$3::uuid
            `, fixture.payoutID, uuid.NewString(), lease.Token)
			if err != nil {
				t.Fatal(err)
			}
			if tag.RowsAffected() != 1 {
				t.Fatalf("replacement affected %d rows", tag.RowsAffected())
			}
		}

		result, err := verify(provider, lease.Token)
		if result != nil ||
			!errors.Is(err, ErrLeaseFencedVerificationDenied) ||
			provider.calls != 1 {
			t.Fatalf(
				"stale evidence returned: result=%+v calls=%d err=%v",
				result, provider.calls, err,
			)
		}
	})

	t.Run("expires_during_lookup", func(t *testing.T) {
		// Restore the original lease token and sufficient validity
		// so the provider lookup is permitted to begin.
		_, err := fixture.db.Exec(fixture.ctx, `
            UPDATE driver_payout_recovery_leases
            SET lease_token=$2::uuid,
                lease_owner='verification-integration-owner',
                lease_expires_at=clock_timestamp()+interval '90 seconds',
                updated_at=clock_timestamp()
            WHERE payout_id=$1::uuid
        `, fixture.payoutID, lease.Token)
		if err != nil {
			t.Fatal(err)
		}

		provider := &leaseVerificationProvider{details: details}

		provider.beforeReturn = func() {
			tag, err := fixture.db.Exec(fixture.ctx, `
                UPDATE driver_payout_recovery_leases
                SET lease_expires_at=
                    clock_timestamp()-interval '1 second',
                    updated_at=clock_timestamp()
                WHERE payout_id=$1::uuid
                  AND lease_token=$2::uuid
            `, fixture.payoutID, lease.Token)

			if err != nil {
				t.Fatal(err)
			}
			if tag.RowsAffected() != 1 {
				t.Fatalf(
					"expiry update affected %d rows",
					tag.RowsAffected(),
				)
			}
		}

		result, err := verify(provider, lease.Token)

		if result != nil ||
			!errors.Is(err, ErrLeaseFencedVerificationDenied) ||
			provider.calls != 1 {
			t.Fatalf(
				"expired in-flight lease returned evidence: result=%+v calls=%d err=%v",
				result, provider.calls, err,
			)
		}
	})

	t.Run("expired_lease", func(t *testing.T) {
		_, err := fixture.db.Exec(fixture.ctx, `
            UPDATE driver_payout_recovery_leases
            SET lease_expires_at=clock_timestamp()-interval '1 second'
            WHERE payout_id=$1::uuid
        `, fixture.payoutID)
		if err != nil {
			t.Fatal(err)
		}

		provider := &leaseVerificationProvider{details: details}
		result, err := verify(provider, lease.Token)
		if result != nil ||
			!errors.Is(err, ErrLeaseFencedVerificationDenied) ||
			provider.calls != 0 {
			t.Fatalf(
				"expired lease accepted: result=%+v calls=%d err=%v",
				result, provider.calls, err,
			)
		}
	})

	t.Run("financial_state_unchanged", func(t *testing.T) {
		var status string
		var attempts int
		var reference *string

		err := fixture.db.QueryRow(fixture.ctx, `
            SELECT execution_status, attempt_count, provider_reference
            FROM driver_payout_executions
            WHERE payout_id=$1::uuid
        `, fixture.payoutID).Scan(
			&status, &attempts, &reference,
		)
		if err != nil {
			t.Fatal(err)
		}

		var payoutStatus string
		err = fixture.db.QueryRow(fixture.ctx, `
            SELECT payout_status
            FROM driver_payouts
            WHERE id=$1::uuid
        `, fixture.payoutID).Scan(&payoutStatus)
		if err != nil {
			t.Fatal(err)
		}

		if status != "PROCESSING" ||
			attempts != 1 ||
			reference != nil ||
			payoutStatus != "PENDING" {
			t.Fatalf(
				"financial mutation: status=%s attempts=%d reference=%v payout=%s",
				status, attempts, reference, payoutStatus,
			)
		}
	})
}
