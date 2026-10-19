package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FlexCapturePreparation identifies the local financial records
// prepared for a FLEX settlement. It is not proof of capture.
type FlexCapturePreparation struct {
	SettlementID            string
	PaymentID               string
	CaptureTransactionID    string
	CaptureIdempotencyKey   string
	ProviderAuthorizationID string
}

// PrepareFlexSettlementCapture atomically prepares local payment
// records for a claimed FLEX settlement.
//
// It does not contact Stripe, capture funds, mark a payment PAID,
// or mark a settlement CAPTURED.
func PrepareFlexSettlementCapture(
	ctx context.Context,
	db *pgxpool.Pool,
	settlementID string,
) (*FlexCapturePreparation, error) {
	if db == nil {
		return nil, errors.New("FLEX settlement database is required")
	}
	if _, err := uuid.Parse(settlementID); err != nil {
		return nil, ErrFlexSettlementNotFound
	}

	var result *FlexCapturePreparation

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var tripID, provider, providerID, captureKey string
		var amount, currency, status string

		err := tx.QueryRow(ctx, `
			SELECT trip_id, provider, provider_authorization_id,
			       capture_idempotency_key, amount::text,
			       currency, status
			FROM flex_settlements
			WHERE id = $1
			FOR UPDATE
		`, settlementID).Scan(
			&tripID, &provider, &providerID,
			&captureKey, &amount, &currency, &status,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexSettlementNotFound
		}
		if err != nil {
			return fmt.Errorf("lock FLEX settlement: %w", err)
		}
		if status != "PROCESSING" || provider != "STRIPE" {
			return ErrFlexSettlementInvalidState
		}

		// Existing payments are reused only after validating
		// their authoritative identity.
		var paymentID, paymentStatus, paymentMethod string
		var paymentAmount, paymentCurrency string

		err = tx.QueryRow(ctx, `
			SELECT id, status, payment_method, amount::text, currency
			FROM payments
			WHERE trip_id = $1
			FOR UPDATE
		`, tripID).Scan(
			&paymentID, &paymentStatus, &paymentMethod,
			&paymentAmount, &paymentCurrency,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			payment, createErr :=
				NewPaymentRepositoryWithDB(tx).
					CreateFromCompletedTrip(ctx, tripID, "CARD")
			if createErr != nil {
				return fmt.Errorf(
					"create FLEX capture payment: %w", createErr,
				)
			}
			paymentID = payment.ID
			paymentStatus = payment.Status
			paymentMethod = payment.PaymentMethod
			paymentAmount = payment.Amount
			paymentCurrency = payment.Currency
		} else if err != nil {
			return fmt.Errorf("read FLEX capture payment: %w", err)
		}

		if paymentMethod != "CARD" ||
			paymentCurrency != currency ||
			!flexAmountsEqual(paymentAmount, amount) ||
			(paymentStatus != "PENDING" &&
				paymentStatus != "PROCESSING" &&
				paymentStatus != "AUTHORIZED") {
			return errors.New(
				"FLEX capture payment identity or status mismatch",
			)
		}

		// The same provider/idempotency pair must never
		// identify another payment or operation.
		var transactionID, transactionPaymentID string
		var transactionType, transactionStatus string
		var transactionAmount, transactionCurrency string

		err = tx.QueryRow(ctx, `
			SELECT id, payment_id, transaction_type,
			       status, amount::text, currency
			FROM payment_transactions
			WHERE provider = $1
			  AND idempotency_key = $2
			FOR UPDATE
		`, provider, captureKey).Scan(
			&transactionID, &transactionPaymentID,
			&transactionType, &transactionStatus,
			&transactionAmount, &transactionCurrency,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			transactionReference := "flex-capture:" + settlementID

			transaction, createErr :=
				NewPaymentTransactionRepositoryWithDB(tx).
					Create(ctx, repository.CreatePaymentTransactionParams{
						PaymentID:            paymentID,
						TransactionReference: transactionReference,
						Provider:             provider,
						IdempotencyKey:       &captureKey,
						TransactionType:      "CAPTURE",
						Amount:               amount,
						Currency:             currency,
					})
			if createErr != nil {
				return fmt.Errorf(
					"create FLEX capture transaction: %w",
					createErr,
				)
			}

			transactionID = transaction.ID
			transactionPaymentID = transaction.PaymentID
			transactionType = transaction.TransactionType
			transactionStatus = transaction.Status
			transactionAmount = transaction.Amount
			transactionCurrency = transaction.Currency
		} else if err != nil {
			return fmt.Errorf(
				"read FLEX capture transaction: %w", err,
			)
		}

		if transactionPaymentID != paymentID ||
			transactionType != "CAPTURE" ||
			!flexAmountsEqual(transactionAmount, amount) ||
			transactionCurrency != currency ||
			(transactionStatus != "PENDING" &&
				transactionStatus != "PROCESSING") {
			return errors.New(
				"FLEX capture transaction identity or status mismatch",
			)
		}

		_, err = tx.Exec(ctx, `
			UPDATE flex_settlements
			SET payment_id = $2,
			    capture_transaction_id = $3,
			    updated_at = clock_timestamp()
			WHERE id = $1
			  AND status = 'PROCESSING'
		`, settlementID, paymentID, transactionID)
		if err != nil {
			return fmt.Errorf(
				"link FLEX capture financial records: %w", err,
			)
		}

		result = &FlexCapturePreparation{
			SettlementID:            settlementID,
			PaymentID:               paymentID,
			CaptureTransactionID:    transactionID,
			CaptureIdempotencyKey:   captureKey,
			ProviderAuthorizationID: providerID,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// flexAmountsEqual compares decimal monetary values using integer cents.
func flexAmountsEqual(a, b string) bool {
	first, err := flexDecimalToCents(a)
	if err != nil {
		return false
	}

	second, err := flexDecimalToCents(b)
	if err != nil {
		return false
	}

	return first == second
}
