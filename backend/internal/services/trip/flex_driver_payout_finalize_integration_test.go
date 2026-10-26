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

func TestFlexDriverPayoutExecutionFinalizationIntegration(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	settlement, err := postgresrepo.CreateFlexSettlementFromCompletedTrip(
		ctx, db, lifecycle.TripID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: %v %v", claimed, err)
	}

	capture, err := postgresrepo.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatal(err)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, capture.ProviderAuthorizationID,
	); err != nil {
		t.Fatal(err)
	}

	var earningID, driverID, companyID string
	err = db.QueryRow(ctx, `
        SELECT id, driver_id, company_id
        FROM driver_earnings
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningID, &driverID, &companyID,
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	reference := "connect-finalize-" + uuid.NewString()

	// Cleanup runs before the FLEX lifecycle cleanup.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer cleanupCancel()

		for _, query := range []string{
			`DELETE FROM driver_payout_executions
             WHERE payout_id IN (
                 SELECT id FROM driver_payouts
                 WHERE payout_reference = $1
             )`,
			`DELETE FROM driver_payout_items
             WHERE payout_id IN (
                 SELECT id FROM driver_payouts
                 WHERE payout_reference = $1
             )`,
			`DELETE FROM driver_payouts
             WHERE payout_reference = $1`,
		} {
			if _, err := db.Exec(cleanupCtx, query, reference); err != nil {
				t.Errorf("payout cleanup: %v", err)
			}
		}
	})

	payout, err := postgresrepo.PrepareFlexDriverPayout(
		ctx, db,
		postgresrepo.PrepareDriverPayoutParams{
			DriverID:        driverID,
			CompanyID:       companyID,
			PayoutReference: reference,
			PeriodStart:     now.Add(-24 * time.Hour),
			PeriodEnd:       now.Add(time.Hour),
		},
	)
	if err != nil {
		t.Fatalf("prepare payout: %v", err)
	}
	if payout == nil || payout.ItemCount != 1 {
		t.Fatalf("invalid payout: %+v", payout)
	}

	const provider = "TEST_PROVIDER"
	providerReference := "synthetic-transfer-" + uuid.NewString()

	execution, err := postgresrepo.CreateDriverPayoutExecution(
		ctx, db,
		postgresrepo.CreateDriverPayoutExecutionParams{
			PayoutID: payout.ID,
			Provider: provider,
		},
	)
	if err != nil || execution == nil {
		t.Fatalf("create execution: %+v %v", execution, err)
	}

	t.Run("premature_finalization_rejected", func(t *testing.T) {
		err := postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, db, payout.ID, provider, providerReference,
		)
		if !errors.Is(
			err, postgresrepo.ErrDriverPayoutExecutionConflict,
		) {
			t.Fatalf("expected premature conflict: %v", err)
		}
	})

	_, claimed, err = postgresrepo.ClaimDriverPayoutExecution(
		ctx, db, payout.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim payout execution: %v %v", claimed, err)
	}

	t.Run("incorrect_provider_rejected", func(t *testing.T) {
		err := postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, db, payout.ID, "WRONG_PROVIDER", providerReference,
		)
		if !errors.Is(
			err, postgresrepo.ErrDriverPayoutExecutionConflict,
		) {
			t.Fatalf("expected provider conflict: %v", err)
		}
	})

	t.Run("concurrent_success_finalization", func(t *testing.T) {
		const callers = 6
		results := make([]error, callers)
		var wg sync.WaitGroup
		start := make(chan struct{})

		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				<-start
				results[index] =
					postgresrepo.FinalizeDriverPayoutExecutionSuccess(
						ctx, db, payout.ID, provider, providerReference,
					)
			}(i)
		}

		close(start)
		wg.Wait()

		for i, err := range results {
			if err != nil {
				t.Fatalf("concurrent finalizer %d: %v", i, err)
			}
		}
	})

	t.Run("financial_state_verified", func(t *testing.T) {
		var executionStatus, payoutStatus, earningStatus string
		var amount string
		var executionCount, itemCount int

		err := db.QueryRow(ctx, `
            SELECT
                x.execution_status,
                p.payout_status,
                e.settlement_status,
                p.net_amount::text,
                (SELECT COUNT(*)
                 FROM driver_payout_executions
                 WHERE payout_id = p.id),
                (SELECT COUNT(*)
                 FROM driver_payout_items
                 WHERE payout_id = p.id)
            FROM driver_payouts p
            JOIN driver_payout_items i ON i.payout_id = p.id
            JOIN driver_earnings e ON e.id = i.earning_id
            JOIN driver_payout_executions x ON x.payout_id = p.id
            WHERE p.id = $1 AND e.id = $2
        `, payout.ID, earningID).Scan(
			&executionStatus, &payoutStatus, &earningStatus,
			&amount, &executionCount, &itemCount,
		)
		if err != nil {
			t.Fatal(err)
		}

		if executionStatus != "SUCCEEDED" ||
			payoutStatus != "PAID" ||
			earningStatus != "PAID" ||
			!postgresrepoAmountsEqualForPayoutTest(amount, "21.65") ||
			executionCount != 1 ||
			itemCount != 1 {
			t.Fatalf(
				"incorrect payout state: execution=%s payout=%s earning=%s amount=%s executions=%d items=%d",
				executionStatus, payoutStatus, earningStatus,
				amount, executionCount, itemCount,
			)
		}
	})

	t.Run("idempotent_replay", func(t *testing.T) {
		if err := postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, db, payout.ID, provider, providerReference,
		); err != nil {
			t.Fatalf("replay failed: %v", err)
		}
	})

	t.Run("conflicting_reference_rejected", func(t *testing.T) {
		err := postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, db, payout.ID, provider, "different-transfer-reference",
		)
		if !errors.Is(
			err, postgresrepo.ErrDriverPayoutExecutionConflict,
		) {
			t.Fatalf("expected reference conflict: %v", err)
		}
	})
}
