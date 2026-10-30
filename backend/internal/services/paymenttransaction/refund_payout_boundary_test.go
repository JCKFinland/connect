package paymenttransaction

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestRefundCannotStartAfterDriverEarningPayoutCommitment(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("unsafe integration database")
	}

	ctx := context.Background()
	fixture := newPaymentTransactionTestFixture(t)
	db := fixture.DB

	var actualDB string
	if err := db.QueryRow(ctx, "SELECT current_database()").Scan(&actualDB); err != nil {
		t.Fatal(err)
	}
	if actualDB != "connect_integration_test" {
		t.Fatalf("unsafe database: %s", actualDB)
	}

	_, err := db.Exec(ctx, `
        UPDATE payments
        SET status = 'PAID',
            paid_at = NOW(),
            updated_at = NOW()
        WHERE id = $1
    `, fixture.PaymentID)
	if err != nil {
		t.Fatalf("mark payment paid: %v", err)
	}

	earning, err := postgresrepo.NewDriverEarningRepository(db).
		CreateFromPaidPayment(ctx, fixture.PaymentID)
	if err != nil {
		t.Fatalf("create driver earning: %v", err)
	}

	payoutID := uuid.NewString()
	payoutReference := "refund-boundary-" + uuid.NewString()

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_items WHERE payout_id = $1
        `, payoutID); err != nil {
			t.Errorf("cleanup payout items: %v", err)
		}

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_payouts WHERE id = $1
        `, payoutID); err != nil {
			t.Errorf("cleanup payout: %v", err)
		}

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_earnings WHERE id = $1
        `, earning.ID); err != nil {
			t.Errorf("cleanup earning: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
        INSERT INTO driver_payouts (
            id, driver_id, company_id, payout_reference,
            period_start, period_end,
            gross_amount, deductions, adjustments,
            net_amount, currency, payout_method, payout_status
        )
        SELECT
            $1, driver_id, company_id, $2,
            NOW() - INTERVAL '2 days',
            NOW() - INTERVAL '1 day',
            gross_amount, commission_amount, 0,
            net_amount, currency, 'BANK_TRANSFER', 'PROCESSING'
        FROM driver_earnings
        WHERE id = $3
    `, payoutID, payoutReference, earning.ID)
	if err != nil {
		t.Fatalf("create payout commitment: %v", err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO driver_payout_items (
            payout_id, earning_id, amount
        )
        VALUES ($1, $2, $3::numeric)
    `, payoutID, earning.ID, earning.NetAmount)
	if err != nil {
		t.Fatalf("link earning to payout: %v", err)
	}

	_, err = db.Exec(ctx, `
        UPDATE driver_earnings
        SET settlement_status = 'PROCESSING',
            updated_at = NOW()
        WHERE id = $1
    `, earning.ID)
	if err != nil {
		t.Fatalf("mark earning processing: %v", err)
	}

	var linkedCount int
	err = db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items i
        JOIN driver_payouts p ON p.id = i.payout_id
        JOIN driver_earnings e ON e.id = i.earning_id
        WHERE e.id = $1
          AND e.payment_id = $2
          AND e.settlement_status = 'PROCESSING'
          AND p.payout_status = 'PROCESSING'
    `, earning.ID, fixture.PaymentID).Scan(&linkedCount)
	if err != nil {
		t.Fatalf("verify payout commitment: %v", err)
	}
	if linkedCount != 1 {
		t.Fatalf("expected one committed payout item, got %d", linkedCount)
	}

	service := NewService(db)

	refund, err := service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID:       fixture.PaymentID,
			Provider:        "TEST_PROVIDER",
			IdempotencyKey:  uuid.NewString(),
			TransactionType: TypeRefund,
			Amount:          "0.40",
		},
	)

	if err == nil {
		t.Fatalf(
			"FINANCIAL BOUNDARY VIOLATION: refund %s was initiated while earning %s was committed to payout %s",
			refund.ID,
			earning.ID,
			payoutID,
		)
	}

	var refundCount int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE payment_id = $1
          AND transaction_type = 'REFUND'
    `, fixture.PaymentID).Scan(&refundCount); err != nil {
		t.Fatalf("count refund transactions: %v", err)
	}
	if refundCount != 0 {
		t.Fatalf("expected no refund transaction, found %d", refundCount)
	}
}
