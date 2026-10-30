package trip

import (
	"context"
	"os"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestFlexPayoutPreparationWaitsForFinancialBoundaryLock(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement test database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx, "SELECT current_database()",
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
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	capture, err := postgresrepo.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	)
	if err != nil || capture == nil || capture.PaymentID == "" {
		t.Fatalf("prepare capture: capture=%+v err=%v", capture, err)
	}

	if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("mark capture requested: %v", err)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, capture.ProviderAuthorizationID,
	); err != nil {
		t.Fatalf("finalize capture: %v", err)
	}

	var earningID, driverID, companyID string
	if err := db.QueryRow(ctx, `
        SELECT id, driver_id, company_id
        FROM driver_earnings
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningID, &driverID, &companyID,
	); err != nil {
		t.Fatalf("read earning: %v", err)
	}

	reference := "connect-payout-lock-wait-" + uuid.NewString()
	now := time.Now().UTC()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer cleanupCancel()

		_, _ = db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_items
            WHERE payout_id IN (
                SELECT id FROM driver_payouts
                WHERE payout_reference = $1
            )
        `, reference)

		_, _ = db.Exec(cleanupCtx, `
            DELETE FROM driver_payouts
            WHERE payout_reference = $1
        `, reference)
	})

	// Hold the same financial boundary lock used by refund
	// initiation and payout preparation.
	lockTx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin financial lock transaction: %v", err)
	}

	lockHeld := true
	defer func() {
		if lockHeld {
			rollbackCtx, rollbackCancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer rollbackCancel()
			_ = lockTx.Rollback(rollbackCtx)
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		lockTx,
		"payment-financial-boundary:"+capture.PaymentID,
	); err != nil {
		t.Fatalf("acquire payment financial lock: %v", err)
	}

	type payoutOutcome struct {
		payout *postgresrepo.PreparedDriverPayout
		err    error
	}

	result := make(chan payoutOutcome, 1)
	started := make(chan struct{})

	go func() {
		close(started)

		payout, err := postgresrepo.PrepareFlexDriverPayout(
			ctx,
			db,
			postgresrepo.PrepareDriverPayoutParams{
				DriverID:        driverID,
				CompanyID:       companyID,
				PayoutReference: reference,
				PeriodStart:     now.Add(-24 * time.Hour),
				PeriodEnd:       now.Add(time.Hour),
			},
		)

		result <- payoutOutcome{payout: payout, err: err}
	}()

	<-started

	// A payout must not complete while its payment-scoped
	// financial boundary lock remains held.
	select {
	case outcome := <-result:
		t.Fatalf(
			"payout completed before financial lock release: payout=%+v err=%v",
			outcome.payout, outcome.err,
		)
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		t.Fatalf("timed out while holding financial lock: %v", ctx.Err())
	}

	var payoutCount int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE earning_id = $1
    `, earningID).Scan(&payoutCount); err != nil {
		t.Fatalf("count payout items while locked: %v", err)
	}

	if payoutCount != 0 {
		t.Fatalf(
			"payout item committed while financial lock held: %d",
			payoutCount,
		)
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("release financial boundary lock: %v", err)
	}
	lockHeld = false

	var payout *postgresrepo.PreparedDriverPayout

	select {
	case outcome := <-result:
		if outcome.err != nil {
			t.Fatalf("payout did not resume successfully: %v", outcome.err)
		}
		payout = outcome.payout
	case <-ctx.Done():
		t.Fatalf("payout did not resume after lock release: %v", ctx.Err())
	}

	if payout == nil || payout.ItemCount != 1 {
		t.Fatalf(
			"expected one payout item after lock release, got %+v",
			payout,
		)
	}

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE earning_id = $1 AND payout_id = $2
    `, earningID, payout.ID).Scan(&payoutCount); err != nil {
		t.Fatalf("count committed payout items: %v", err)
	}

	if payoutCount != 1 {
		t.Fatalf(
			"expected exactly one committed payout item, got %d",
			payoutCount,
		)
	}

	var earningStatus string
	if err := db.QueryRow(ctx, `
        SELECT settlement_status
        FROM driver_earnings
        WHERE id = $1
    `, earningID).Scan(&earningStatus); err != nil {
		t.Fatalf("read earning status: %v", err)
	}

	if earningStatus != "PROCESSING" {
		t.Fatalf(
			"expected PROCESSING earning after payout, got %s",
			earningStatus,
		)
	}
}
