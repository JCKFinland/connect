package paymenttransaction

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestRefundInitiationWaitsForFinancialBoundaryLock(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("unsafe integration database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

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

	lockTx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
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
		"payment-financial-boundary:"+fixture.PaymentID,
	); err != nil {
		t.Fatalf("acquire financial boundary lock: %v", err)
	}

	result := make(chan error, 1)
	started := make(chan struct{})

	go func() {
		close(started)

		_, err := NewService(db).InitiateOperation(
			ctx,
			InitiateOperationRequest{
				PaymentID:       fixture.PaymentID,
				Provider:        "TEST_PROVIDER",
				IdempotencyKey:  uuid.NewString(),
				TransactionType: TypeRefund,
				Amount:          "0.40",
			},
		)

		result <- err
	}()

	<-started

	// Confirm that the refund has not completed while the financial
	// boundary lock is held. This is a bounded observation, not the
	// sole proof that the lock was acquired.
	select {
	case err := <-result:
		t.Fatalf("refund completed before lock release: %v", err)
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		t.Fatalf("timed out before lock release: %v", ctx.Err())
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("release financial boundary lock: %v", err)
	}
	lockHeld = false

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("refund did not resume successfully: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("refund failed to resume after lock release: %v", ctx.Err())
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

	if refundCount != 1 {
		t.Fatalf("expected exactly one refund, got %d", refundCount)
	}

	// The transaction must remain usable after the lock contention.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("financial lock test exceeded deadline")
	}
}
