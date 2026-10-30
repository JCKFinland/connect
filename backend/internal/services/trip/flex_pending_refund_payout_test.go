package trip

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
	"github.com/google/uuid"
)

func TestFlexPendingRefundPreventsPayoutPreparation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
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
		t.Fatalf("create FLEX settlement: %v", err)
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
	if err != nil {
		t.Fatalf("prepare capture: %v", err)
	}
	if capture == nil || capture.PaymentID == "" {
		t.Fatal("missing authoritative captured payment ID")
	}

	if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("mark capture requested: %v", err)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, capture.ProviderAuthorizationID,
	); err != nil {
		t.Fatalf("finalize simulated capture: %v", err)
	}

	var driverID, companyID, earningID, earningPaymentID string
	var earningStatus string

	if err := db.QueryRow(ctx, `
        SELECT e.id, e.driver_id, e.company_id,
               e.payment_id::text, e.settlement_status
        FROM driver_earnings e
        WHERE e.trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningID,
		&driverID,
		&companyID,
		&earningPaymentID,
		&earningStatus,
	); err != nil {
		t.Fatalf("read captured earning: %v", err)
	}

	if earningPaymentID != capture.PaymentID {
		t.Fatalf(
			"earning payment mismatch: earning=%s capture=%s",
			earningPaymentID, capture.PaymentID,
		)
	}

	if earningStatus != "PENDING" {
		t.Fatalf("expected PENDING earning, got %s", earningStatus)
	}

	refund, err := paymenttransaction.NewService(db).InitiateOperation(
		ctx,
		paymenttransaction.InitiateOperationRequest{
			PaymentID:       capture.PaymentID,
			Provider:        "TEST_PROVIDER",
			IdempotencyKey:  uuid.NewString(),
			TransactionType: paymenttransaction.TypeRefund,
			Amount:          "0.40",
		},
	)
	if err != nil {
		t.Fatalf("initiate pending refund: %v", err)
	}

	if refund == nil || refund.Status != paymenttransaction.StatusPending {
		t.Fatalf("expected pending refund, got %+v", refund)
	}

	reference := "connect-refund-boundary-" + uuid.NewString()

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

	now := time.Now().UTC()

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

	if !errors.Is(err, postgresrepo.ErrDriverPayoutNoEligibleEarnings) {
		t.Fatalf(
			"expected pending refund to block payout, payout=%+v err=%v",
			payout, err,
		)
	}

	var payoutItems int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE earning_id = $1
    `, earningID).Scan(&payoutItems); err != nil {
		t.Fatalf("count payout items: %v", err)
	}

	if payoutItems != 0 {
		t.Fatalf("pending refund earning was claimed by %d payout items", payoutItems)
	}

	if err := db.QueryRow(ctx, `
        SELECT settlement_status
        FROM driver_earnings
        WHERE id = $1
    `, earningID).Scan(&earningStatus); err != nil {
		t.Fatalf("recheck earning: %v", err)
	}

	if earningStatus != "PENDING" {
		t.Fatalf("earning changed despite refund: %s", earningStatus)
	}

	// A delayed successful refund must not make this earning
	// eligible for payout after the pending refund blocked it.
	completedRefund, err := paymenttransaction.NewService(db).ApplyResult(
		ctx,
		refund.ID,
		paymenttransaction.ApplyResultRequest{
			Status: paymenttransaction.StatusSuccess,
		},
	)
	if err != nil {
		t.Fatalf("apply delayed refund success: %v", err)
	}
	if completedRefund == nil ||
		completedRefund.Status != paymenttransaction.StatusSuccess {
		t.Fatalf("expected successful refund, got %+v", completedRefund)
	}

	var paymentStatus string
	if err := db.QueryRow(ctx, `
        SELECT status
        FROM payments
        WHERE id = $1
    `, capture.PaymentID).Scan(&paymentStatus); err != nil {
		t.Fatalf("read payment after delayed refund: %v", err)
	}
	if paymentStatus != "PARTIALLY_REFUNDED" {
		t.Fatalf("expected PARTIALLY_REFUNDED, got %s", paymentStatus)
	}

	lateReference := "connect-refund-completed-" + uuid.NewString()

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
        `, lateReference)

		_, _ = db.Exec(cleanupCtx, `
            DELETE FROM driver_payouts
            WHERE payout_reference = $1
        `, lateReference)
	})

	latePayout, err := postgresrepo.PrepareFlexDriverPayout(
		ctx,
		db,
		postgresrepo.PrepareDriverPayoutParams{
			DriverID:        driverID,
			CompanyID:       companyID,
			PayoutReference: lateReference,
			PeriodStart:     now.Add(-24 * time.Hour),
			PeriodEnd:       now.Add(time.Hour),
		},
	)
	if !errors.Is(err, postgresrepo.ErrDriverPayoutNoEligibleEarnings) {
		t.Fatalf(
			"expected successful refund to block payout, payout=%+v err=%v",
			latePayout, err,
		)
	}

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE earning_id = $1
    `, earningID).Scan(&payoutItems); err != nil {
		t.Fatalf("count payout items after refund success: %v", err)
	}
	if payoutItems != 0 {
		t.Fatalf("refunded earning claimed by %d payout items", payoutItems)
	}

	if err := db.QueryRow(ctx, `
        SELECT settlement_status
        FROM driver_earnings
        WHERE id = $1
    `, earningID).Scan(&earningStatus); err != nil {
		t.Fatalf("read earning after refund success: %v", err)
	}
	if earningStatus != "PENDING" {
		t.Fatalf("earning changed after refund success: %s", earningStatus)
	}
}
