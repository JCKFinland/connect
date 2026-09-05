package paymenttransaction

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestInitiateOperationIdempotencyAndSingleInflight(
	t *testing.T,
) {
	ctx := context.Background()

	fixture :=
		newPaymentTransactionTestFixture(t)

	service := NewService(fixture.DB)

	idempotencyKey := uuid.NewString()

	first, err := service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID: fixture.PaymentID,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: idempotencyKey,

			TransactionType: TypeSale,
		},
	)
	if err != nil {
		t.Fatalf(
			"initiate SALE: %v",
			err,
		)
	}

	if first.Status != StatusPending {
		t.Fatalf(
			"expected PENDING, got %s",
			first.Status,
		)
	}

	if first.Amount != fixture.Payment.Amount {
		t.Fatalf(
			"expected authoritative amount %s, got %s",
			fixture.Payment.Amount,
			first.Amount,
		)
	}

	if first.Currency != fixture.Payment.Currency {
		t.Fatalf(
			"expected authoritative currency %s, got %s",
			fixture.Payment.Currency,
			first.Currency,
		)
	}

	replayed, err := service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID: fixture.PaymentID,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: idempotencyKey,

			TransactionType: TypeSale,
		},
	)
	if err != nil {
		t.Fatalf(
			"replay SALE: %v",
			err,
		)
	}

	if replayed.ID != first.ID {
		t.Fatalf(
			"idempotent replay returned %s want %s",
			replayed.ID,
			first.ID,
		)
	}

	_, err = service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID: fixture.PaymentID,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: idempotencyKey,

			TransactionType: TypeAuthorize,
		},
	)

	if !errors.Is(
		err,
		ErrPaymentOperationIdempotencyConflict,
	) {
		t.Fatalf(
			"expected idempotency conflict, got %v",
			err,
		)
	}

	_, err = service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID: fixture.PaymentID,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: uuid.NewString(),

			TransactionType: TypeAuthorize,
		},
	)

	if err == nil {
		t.Fatal(
			"expected second in-flight operation to fail",
		)
	}
}

func TestInitiateRefundValidatesRemainingAmount(
	t *testing.T,
) {
	ctx := context.Background()

	fixture :=
		newPaymentTransactionTestFixture(t)

	service := NewService(fixture.DB)

	// Put the aggregate payment into PAID so REFUND is a legal
	// provider operation.
	_, err := fixture.DB.Exec(
		ctx,
		`
			UPDATE payments
			SET
				status = 'PAID',
				paid_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
		`,
		fixture.PaymentID,
	)
	if err != nil {
		t.Fatalf(
			"prepare paid payment: %v",
			err,
		)
	}

	firstRefund, err :=
		service.InitiateOperation(
			ctx,
			InitiateOperationRequest{
				PaymentID: fixture.PaymentID,

				Provider: "TEST_PROVIDER",

				IdempotencyKey: uuid.NewString(),

				TransactionType: TypeRefund,

				Amount: "0.40",
			},
		)
	if err != nil {
		t.Fatalf(
			"initiate first refund: %v",
			err,
		)
	}

	if firstRefund.Amount != "0.40" {
		t.Fatalf(
			"expected refund amount 0.40, got %s",
			firstRefund.Amount,
		)
	}

	// The first refund is still PENDING, so a second provider operation
	// must be blocked by the single-inflight invariant.
	_, err = service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID: fixture.PaymentID,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: uuid.NewString(),

			TransactionType: TypeRefund,

			Amount: "0.60",
		},
	)

	if err == nil {
		t.Fatal(
			"expected second in-flight refund to fail",
		)
	}

	// Mark the first refund SUCCESS so the next refund may be initiated.
	providerTransactionID :=
		"provider_" + uuid.NewString()

	_, err = service.ApplyResult(
		ctx,
		firstRefund.ID,
		ApplyResultRequest{
			Status: StatusSuccess,

			ProviderTransactionID: &providerTransactionID,
		},
	)
	if err != nil {
		t.Fatalf(
			"complete first refund: %v",
			err,
		)
	}

	secondRefund, err :=
		service.InitiateOperation(
			ctx,
			InitiateOperationRequest{
				PaymentID: fixture.PaymentID,

				Provider: "TEST_PROVIDER",

				IdempotencyKey: uuid.NewString(),

				TransactionType: TypeRefund,

				Amount: "0.60",
			},
		)
	if err != nil {
		t.Fatalf(
			"initiate remaining refund: %v",
			err,
		)
	}

	if secondRefund.Amount != "0.60" {
		t.Fatalf(
			"expected remaining refund amount 0.60, got %s",
			secondRefund.Amount,
		)
	}

	// Remove the newly-created pending refund so we can test
	// amount validation without the single-inflight constraint
	// being the reason for rejection.
	_, err = fixture.DB.Exec(
		ctx,
		`
			DELETE FROM payment_transactions
			WHERE id = $1
		`,
		secondRefund.ID,
	)
	if err != nil {
		t.Fatalf(
			"remove pending second refund: %v",
			err,
		)
	}

	_, err = service.InitiateOperation(
		ctx,
		InitiateOperationRequest{
			PaymentID: fixture.PaymentID,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: uuid.NewString(),

			TransactionType: TypeRefund,

			Amount: "0.61",
		},
	)

	if !errors.Is(
		err,
		ErrPaymentOperationAmountInvalid,
	) {
		t.Fatalf(
			"expected ErrPaymentOperationAmountInvalid, got %v",
			err,
		)
	}
}
