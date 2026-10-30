package trip

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
	"github.com/google/uuid"
)

func TestFlexRefundAndPayoutCannotBothCommit(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
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
		t.Fatal(err)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, capture.ProviderAuthorizationID,
	); err != nil {
		t.Fatal(err)
	}

	var earningID, driverID, companyID string
	if err := db.QueryRow(ctx, `
        SELECT id, driver_id, company_id
        FROM driver_earnings
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningID, &driverID, &companyID,
	); err != nil {
		t.Fatal(err)
	}

	reference := "connect-refund-payout-race-" + uuid.NewString()
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

	start := make(chan struct{})
	var wg sync.WaitGroup

	var refundErr error
	var refundWon bool

	var payoutErr error
	var payoutResult *postgresrepo.PreparedDriverPayout

	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start

		refundResult, err := paymenttransaction.NewService(db).InitiateOperation(ctx, paymenttransaction.InitiateOperationRequest{
			PaymentID:       capture.PaymentID,
			Provider:        "TEST_PROVIDER",
			IdempotencyKey:  uuid.NewString(),
			TransactionType: paymenttransaction.TypeRefund,
			Amount:          "0.40",
		})
		refundErr = err
		refundWon = err == nil && refundResult != nil
	}()

	go func() {
		defer wg.Done()
		<-start

		payoutResult, payoutErr = postgresrepo.PrepareFlexDriverPayout(
			ctx, db, postgresrepo.PrepareDriverPayoutParams{
				DriverID:        driverID,
				CompanyID:       companyID,
				PayoutReference: reference,
				PeriodStart:     now.Add(-24 * time.Hour),
				PeriodEnd:       now.Add(time.Hour),
			},
		)
	}()

	close(start)
	wg.Wait()

	payoutWon := payoutErr == nil && payoutResult != nil

	if refundWon == payoutWon {
		t.Fatalf(
			"expected exactly one financial winner: refund_won=%v err=%v payout=%+v err=%v",
			refundWon, refundErr, payoutResult, payoutErr,
		)
	}

	if !refundWon && !errors.Is(
		refundErr, paymenttransaction.ErrInvalidPaymentOperation,
	) {
		t.Fatalf("unexpected refund rejection: %v", refundErr)
	}

	if !payoutWon && !errors.Is(
		payoutErr, postgresrepo.ErrDriverPayoutNoEligibleEarnings,
	) {
		t.Fatalf("unexpected payout rejection: %v", payoutErr)
	}

	var refundCount, payoutItemCount int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE payment_id = $1
          AND transaction_type = 'REFUND'
          AND status IN ('PENDING', 'PROCESSING', 'SUCCESS')
    `, capture.PaymentID).Scan(&refundCount); err != nil {
		t.Fatal(err)
	}

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE earning_id = $1
    `, earningID).Scan(&payoutItemCount); err != nil {
		t.Fatal(err)
	}

	if refundCount+payoutItemCount != 1 {
		t.Fatalf(
			"financial boundary violated: refunds=%d payout_items=%d",
			refundCount, payoutItemCount,
		)
	}

	t.Logf(
		"financial winner: refund=%v payout=%v",
		refundWon, payoutWon,
	)
}
