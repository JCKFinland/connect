package trip

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type flexPayoutFinalizationFixture struct {
	DB        *pgxpool.Pool
	PayoutID  string
	EarningID string
	Reference string
}

func prepareFlexPayoutFinalizationFixture(
	t *testing.T,
	ctx context.Context,
) flexPayoutFinalizationFixture {
	t.Helper()

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

	settlement, err :=
		postgresrepo.CreateFlexSettlementFromCompletedTrip(
			ctx, db, lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexSettlementProcessing(
			ctx, db, settlement.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: %v %v", claimed, err)
	}

	capture, err :=
		postgresrepo.PrepareFlexSettlementCapture(
			ctx, db, settlement.ID,
		)
	if err != nil {
		t.Fatalf("prepare capture: %v", err)
	}

	if capture == nil ||
		capture.ProviderAuthorizationID == "" {
		t.Fatal("missing synthetic capture identity")
	}

	if _, err :=
		postgresrepo.MarkFlexSettlementCaptureRequested(
			ctx, db, settlement.ID,
		); err != nil {
		t.Fatalf("mark capture requested: %v", err)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID,
		capture.ProviderAuthorizationID,
	); err != nil {
		t.Fatalf("finalize synthetic capture: %v", err)
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
		t.Fatalf("read earning: %v", err)
	}

	reference := "connect-finalize-recovery-" + uuid.NewString()

	// Cleanup is registered after the trip lifecycle fixture,
	// so payout records are removed before the trip records.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer cancel()

		queries := []string{
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
		}

		for _, query := range queries {
			if _, err := db.Exec(
				cleanupCtx, query, reference,
			); err != nil {
				t.Errorf("cleanup payout: %v", err)
			}
		}
	})

	now := time.Now().UTC()

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
		t.Fatalf("unexpected payout: %+v", payout)
	}

	execution, err :=
		postgresrepo.CreateDriverPayoutExecution(
			ctx, db,
			postgresrepo.CreateDriverPayoutExecutionParams{
				PayoutID: payout.ID,
				Provider: "TEST_PROVIDER",
			},
		)
	if err != nil || execution == nil {
		t.Fatalf("create execution: %+v %v", execution, err)
	}

	_, claimed, err =
		postgresrepo.ClaimDriverPayoutExecution(
			ctx, db, payout.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("claim execution: %v %v", claimed, err)
	}

	return flexPayoutFinalizationFixture{
		DB:        db,
		PayoutID:  payout.ID,
		EarningID: earningID,
		Reference: reference,
	}
}

func assertFlexPayoutFinalizationState(
	t *testing.T,
	ctx context.Context,
	fixture flexPayoutFinalizationFixture,
	expectedExecution string,
	expectedPayout string,
	expectedEarning string,
	expectedAttempts int,
) {
	t.Helper()

	var executionStatus, payoutStatus, earningStatus string
	var attempts int
	var executionReference, payoutReference *string
	var executionConfirmed, payoutProcessed, earningSettled bool

	err := fixture.DB.QueryRow(ctx, `
        SELECT
            x.execution_status,
            p.payout_status,
            e.settlement_status,
            x.attempt_count,
            x.provider_reference,
            p.provider_reference,
            x.provider_confirmed_at IS NOT NULL,
            p.processed_at IS NOT NULL,
            e.settled_at IS NOT NULL
        FROM driver_payout_executions x
        JOIN driver_payouts p ON p.id = x.payout_id
        JOIN driver_payout_items i ON i.payout_id = p.id
        JOIN driver_earnings e ON e.id = i.earning_id
        WHERE p.id = $1 AND e.id = $2
    `, fixture.PayoutID, fixture.EarningID).Scan(
		&executionStatus,
		&payoutStatus,
		&earningStatus,
		&attempts,
		&executionReference,
		&payoutReference,
		&executionConfirmed,
		&payoutProcessed,
		&earningSettled,
	)
	if err != nil {
		t.Fatal(err)
	}

	if executionStatus != expectedExecution ||
		payoutStatus != expectedPayout ||
		earningStatus != expectedEarning ||
		attempts != expectedAttempts {
		t.Fatalf(
			"unexpected states: execution=%s payout=%s earning=%s attempts=%d",
			executionStatus, payoutStatus, earningStatus, attempts,
		)
	}

	successful := expectedExecution == "SUCCEEDED"

	if successful {
		if executionReference == nil ||
			payoutReference == nil ||
			*executionReference != *payoutReference ||
			!executionConfirmed ||
			!payoutProcessed ||
			!earningSettled {
			t.Fatalf(
				"successful payout missing confirmation: executionRef=%v payoutRef=%v",
				executionReference, payoutReference,
			)
		}
	} else {
		if executionReference != nil ||
			payoutReference != nil ||
			executionConfirmed ||
			payoutProcessed ||
			earningSettled {
			t.Fatal("financial state changed before confirmed success")
		}
	}
}

func TestFlexDriverPayoutFinalizationRecoveryAndRollback(
	t *testing.T,
) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 120*time.Second,
	)
	defer cancel()

	t.Run("reconciliation_recovery", func(t *testing.T) {
		fixture := prepareFlexPayoutFinalizationFixture(t, ctx)

		result, err :=
			postgresrepo.MarkDriverPayoutRequiresReconciliation(
				ctx, fixture.DB, fixture.PayoutID,
				"synthetic provider timeout",
			)
		if err != nil || result == nil ||
			result.Status != "REQUIRES_RECONCILIATION" {
			t.Fatalf("mark reconciliation: %+v %v", result, err)
		}

		assertFlexPayoutFinalizationState(
			t, ctx, fixture,
			"REQUIRES_RECONCILIATION",
			"PENDING",
			"PROCESSING",
			1,
		)

		providerReference := "synthetic-recovered-" + uuid.NewString()

		if err := postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, fixture.DB, fixture.PayoutID,
			"TEST_PROVIDER", providerReference,
		); err != nil {
			t.Fatalf("finalize recovered payout: %v", err)
		}

		assertFlexPayoutFinalizationState(
			t, ctx, fixture,
			"SUCCEEDED", "PAID", "PAID", 1,
		)

		if err := postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, fixture.DB, fixture.PayoutID,
			"TEST_PROVIDER", providerReference,
		); err != nil {
			t.Fatalf("recovered payout replay: %v", err)
		}

		_, claimed, err :=
			postgresrepo.ClaimDriverPayoutExecution(
				ctx, fixture.DB, fixture.PayoutID,
			)
		if err != nil || claimed {
			t.Fatalf(
				"recovered payout reclaimed: claimed=%v err=%v",
				claimed, err,
			)
		}
	})

	t.Run("mismatched_item_rolls_back", func(t *testing.T) {
		fixture := prepareFlexPayoutFinalizationFixture(t, ctx)

		// This modifies only the isolated fixture's payout item.
		// The original amount is restored before fixture cleanup.
		tag, err := fixture.DB.Exec(ctx, `
            UPDATE driver_payout_items
            SET amount = amount + 0.01
            WHERE payout_id = $1
              AND earning_id = $2
        `, fixture.PayoutID, fixture.EarningID)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("corrupt test payout item: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(
				context.Background(), 15*time.Second,
			)
			defer cancel()

			if _, err := fixture.DB.Exec(cleanupCtx, `
                UPDATE driver_payout_items
                SET amount = (
                    SELECT net_amount
                    FROM driver_earnings
                    WHERE id = $2
                )
                WHERE payout_id = $1
                  AND earning_id = $2
            `, fixture.PayoutID, fixture.EarningID); err != nil {
				t.Errorf("restore payout item: %v", err)
			}
		})

		err = postgresrepo.FinalizeDriverPayoutExecutionSuccess(
			ctx, fixture.DB, fixture.PayoutID,
			"TEST_PROVIDER", "synthetic-invalid-"+uuid.NewString(),
		)
		if !errors.Is(
			err, postgresrepo.ErrDriverPayoutExecutionConflict,
		) {
			t.Fatalf("expected financial mismatch conflict: %v", err)
		}

		assertFlexPayoutFinalizationState(
			t, ctx, fixture,
			"PROCESSING", "PENDING", "PROCESSING", 1,
		)
	})
}
