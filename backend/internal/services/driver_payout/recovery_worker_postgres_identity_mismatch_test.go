package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestPostgresPayoutRecoveryWorkerIdentityMismatch(
	t *testing.T,
) {
	tests := []struct {
		name   string
		change func(*postgresrepo.StaleDriverPayoutExecution)
	}{
		{
			name: "idempotency key",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.IdempotencyKey = "incorrect-original-key"
			},
		},
		{
			name: "provider",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.Provider = "INCORRECT_PROVIDER"
			},
		},
		{
			name: "destination",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.DestinationID = "incorrect-destination"
			},
		},
		{
			name: "amount",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.AmountCents++
			},
		},
		{
			name: "currency",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.Currency = "USD"
			},
		},
		{
			name: "status",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.Status = "PENDING"
			},
		},
		{
			name: "attempt count",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.AttemptCount = 2
			},
		},
		{
			name: "request timestamp",
			change: func(c *postgresrepo.StaleDriverPayoutExecution) {
				c.RequestStartedAt =
					c.RequestStartedAt.Add(time.Second)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSubmissionFixture(t)

			// Remove the coordination row before fixture cleanup.
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(
					context.Background(),
					15*time.Second,
				)
				defer cancel()

				_, err := fixture.db.Exec(ctx, `
					DELETE FROM driver_payout_recovery_leases
					WHERE payout_id = $1::uuid
				`, fixture.payoutID)

				if err != nil {
					t.Errorf(
						"recovery lease cleanup: %v",
						err,
					)
				}
			})

			execution, claimed, err :=
				postgresrepo.ClaimDriverPayoutExecution(
					fixture.ctx,
					fixture.db,
					fixture.payoutID,
				)

			if err != nil || !claimed || execution == nil {
				t.Fatalf(
					"claim failed: claimed=%v execution=%+v err=%v",
					claimed,
					execution,
					err,
				)
			}

			candidate :=
				postgresRecoveryAuthoritativeCandidate(
					t,
					fixture,
				)

			tt.change(&candidate)

			fake := &payoutRecoveryFake{
				candidates: []postgresrepo.StaleDriverPayoutExecution{
					candidate,
				},
			}

			worker, err := NewPostgresPayoutRecoveryWorker(
				fixture.db,
				fake,
				time.Minute,
				10,
			)
			if err != nil {
				t.Fatal(err)
			}

			var reported []error

			err = worker.RunOnce(
				context.Background(),
				func(err error) {
					reported = append(reported, err)
				},
			)
			if err != nil {
				t.Fatalf("worker run: %v", err)
			}

			fake.mu.Lock()
			lookups := fake.lookups
			fake.mu.Unlock()

			if lookups != 0 {
				t.Fatalf(
					"unsafe provider lookup: %d",
					lookups,
				)
			}

			if len(reported) != 1 ||
				!errors.Is(
					reported[0],
					postgresrepo.ErrDriverPayoutRecoveryIdentityMismatch,
				) {
				t.Fatalf(
					"expected identity mismatch error, got %v",
					reported,
				)
			}

			var released bool
			var cooldownFuture bool
			var lookupCount int64

			err = fixture.db.QueryRow(fixture.ctx, `
				SELECT
					lease_expires_at <= clock_timestamp(),
					next_lookup_at > clock_timestamp(),
					lookup_count
				FROM driver_payout_recovery_leases
				WHERE payout_id = $1::uuid
			`, fixture.payoutID).Scan(
				&released,
				&cooldownFuture,
				&lookupCount,
			)
			if err != nil {
				t.Fatal(err)
			}

			if !released ||
				!cooldownFuture ||
				lookupCount != 1 {
				t.Fatalf(
					"invalid lease state: released=%v cooldown=%v count=%d",
					released,
					cooldownFuture,
					lookupCount,
				)
			}

			var status string
			var attempts int
			var reference *string

			err = fixture.db.QueryRow(fixture.ctx, `
				SELECT
					execution_status,
					attempt_count,
					provider_reference
				FROM driver_payout_executions
				WHERE payout_id = $1::uuid
			`, fixture.payoutID).Scan(
				&status,
				&attempts,
				&reference,
			)
			if err != nil {
				t.Fatal(err)
			}

			if status != "PROCESSING" ||
				attempts != 1 ||
				reference != nil {
				t.Fatalf(
					"financial state changed: status=%s attempts=%d reference=%v",
					status,
					attempts,
					reference,
				)
			}
		})
	}
}
