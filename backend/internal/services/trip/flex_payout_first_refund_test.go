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

func TestFlexPayoutCommitmentPreventsLaterRefund(t *testing.T) {
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

	reference := "connect-payout-first-" + uuid.NewString()
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

	// Prepare payout first. This establishes an authoritative
	// committed payout claim before refund initiation.
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
	if err != nil {
		t.Fatalf("prepare payout first: %v", err)
	}
	if payout == nil || payout.ItemCount != 1 {
		t.Fatalf("expected committed payout with one item, got %+v", payout)
	}

	var payoutItemCount int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE earning_id = $1
          AND payout_id = $2
    `, earningID, payout.ID).Scan(&payoutItemCount); err != nil {
		t.Fatalf("count committed payout items: %v", err)
	}
	if payoutItemCount != 1 {
		t.Fatalf("expected one committed payout item, got %d", payoutItemCount)
	}

	// Refund initiation must reject a payment whose earning
	// has already been claimed by a committed driver payout.
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
	if !errors.Is(err, paymenttransaction.ErrInvalidPaymentOperation) {
		t.Fatalf(
			"expected payout commitment to reject refund: refund=%+v err=%v",
			refund, err,
		)
	}

	var refundCount int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE payment_id = $1
          AND transaction_type = 'REFUND'
          AND status IN ('PENDING', 'PROCESSING', 'SUCCESS')
    `, capture.PaymentID).Scan(&refundCount); err != nil {
		t.Fatalf("count refund transactions: %v", err)
	}
	if refundCount != 0 {
		t.Fatalf("refund created after payout commitment: %d", refundCount)
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
		t.Fatalf("expected PROCESSING earning, got %s", earningStatus)
	}
}
