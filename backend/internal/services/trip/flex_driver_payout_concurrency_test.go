package trip

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type concurrentPayoutResult struct {
	payout *postgresrepo.PreparedDriverPayout
	err    error
}

func TestFlexDriverPayoutConcurrentClaims(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Skip("requires isolated FLEX settlement test database")
	}

	ctx := context.Background()

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe test database: %s", databaseName)
	}

	settlement, err :=
		postgresrepo.CreateFlexSettlementFromCompletedTrip(
			ctx, db, lifecycle.TripID,
		)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexSettlementProcessing(
			ctx, db, settlement.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("settlement claim: %v %v", claimed, err)
	}

	capture, err :=
		postgresrepo.PrepareFlexSettlementCapture(
			ctx, db, settlement.ID,
		)
	if err != nil {
		t.Fatal(err)
	}

	if _, err :=
		postgresrepo.MarkFlexSettlementCaptureRequested(
			ctx, db, settlement.ID,
		); err != nil {
		t.Fatal(err)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
		capture.ProviderAuthorizationID,
	); err != nil {
		t.Fatal(err)
	}

	var earningID, driverID, companyID string

	err = db.QueryRow(ctx, `
        SELECT id, driver_id, company_id
        FROM driver_earnings
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningID,
		&driverID,
		&companyID,
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	base := postgresrepo.PrepareDriverPayoutParams{
		DriverID:    driverID,
		CompanyID:   companyID,
		PeriodStart: now.Add(-24 * time.Hour),
		PeriodEnd:   now.Add(time.Hour),
	}

	references := []string{
		"connect-concurrent-a-" + uuid.NewString(),
		"connect-concurrent-b-" + uuid.NewString(),
	}

	// Cleanup runs before the existing FLEX trip cleanup.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		for _, reference := range references {
			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM driver_payout_items
                WHERE payout_id IN (
                    SELECT id FROM driver_payouts
                    WHERE payout_reference = $1
                )
            `, reference); err != nil {
				t.Errorf("cleanup payout items: %v", err)
			}

			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM driver_payouts
                WHERE payout_reference = $1
            `, reference); err != nil {
				t.Errorf("cleanup payout: %v", err)
			}
		}
	})

	t.Run("different_references_one_earning", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})

		results := make([]concurrentPayoutResult, 2)

		for i := 0; i < 2; i++ {
			wg.Add(1)

			go func(index int) {
				defer wg.Done()

				params := base
				params.PayoutReference = references[index]

				<-start

				results[index].payout, results[index].err =
					postgresrepo.PrepareFlexDriverPayout(
						ctx, db, params,
					)
			}(i)
		}

		close(start)
		wg.Wait()

		successes := 0
		noEligible := 0
		var winningID string
		var winningReference string

		for i, result := range results {
			if result.err == nil {
				if result.payout == nil {
					t.Fatal("successful payout returned nil")
				}

				successes++
				winningID = result.payout.ID
				winningReference = references[i]
				continue
			}

			if errors.Is(
				result.err,
				postgresrepo.ErrDriverPayoutNoEligibleEarnings,
			) {
				if result.payout != nil {
					t.Fatal("rejected claim returned payout")
				}
				noEligible++
				continue
			}

			t.Fatalf(
				"unexpected concurrent result %d: %v",
				i,
				result.err,
			)
		}

		if successes != 1 || noEligible != 1 {
			t.Fatalf(
				"expected one winner and one rejected claim: successes=%d rejected=%d",
				successes,
				noEligible,
			)
		}

		var payoutCount, itemCount int64

		err := db.QueryRow(ctx, `
            SELECT
                (SELECT COUNT(*)
                 FROM driver_payouts
                 WHERE payout_reference = ANY($1::text[])),
                (SELECT COUNT(*)
                 FROM driver_payout_items
                 WHERE earning_id = $2)
        `, references, earningID).Scan(
			&payoutCount,
			&itemCount,
		)
		if err != nil {
			t.Fatal(err)
		}

		if payoutCount != 1 || itemCount != 1 {
			t.Fatalf(
				"duplicate financial claim: payouts=%d items=%d",
				payoutCount,
				itemCount,
			)
		}

		t.Logf(
			"one earning claimed once: payout=%s reference=%s",
			winningID,
			winningReference,
		)

		// Save the winning reference for the replay scenario.
		base.PayoutReference = winningReference
	})

	t.Run("same_reference_concurrent_replay", func(t *testing.T) {
		if base.PayoutReference == "" {
			t.Fatal("missing winning payout reference")
		}

		var wg sync.WaitGroup
		start := make(chan struct{})

		results := make([]concurrentPayoutResult, 2)

		for i := 0; i < 2; i++ {
			wg.Add(1)

			go func(index int) {
				defer wg.Done()
				<-start

				results[index].payout, results[index].err =
					postgresrepo.PrepareFlexDriverPayout(
						ctx, db, base,
					)
			}(i)
		}

		close(start)
		wg.Wait()

		if results[0].err != nil || results[1].err != nil {
			t.Fatalf(
				"concurrent replay errors: %v / %v",
				results[0].err,
				results[1].err,
			)
		}

		if results[0].payout == nil ||
			results[1].payout == nil ||
			results[0].payout.ID != results[1].payout.ID {
			t.Fatalf(
				"concurrent replay returned inconsistent payouts: %+v %+v",
				results[0].payout,
				results[1].payout,
			)
		}

		var itemCount int64

		if err := db.QueryRow(ctx, `
            SELECT COUNT(*)
            FROM driver_payout_items
            WHERE earning_id = $1
        `, earningID).Scan(&itemCount); err != nil {
			t.Fatal(err)
		}

		if itemCount != 1 {
			t.Fatalf(
				"concurrent replay duplicated earning claim: %d",
				itemCount,
			)
		}

		t.Logf(
			"concurrent replay preserved payout=%s",
			results[0].payout.ID,
		)
	})
}
