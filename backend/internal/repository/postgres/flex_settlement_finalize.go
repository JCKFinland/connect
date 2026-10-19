package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FinalizeFlexSettlementCapture records an externally verified,
// successful Stripe capture. The caller is responsible for verifying
// the provider result before invoking this function.
//
// All local financial changes are committed atomically.
func FinalizeFlexSettlementCapture(
	ctx context.Context,
	db *pgxpool.Pool,
	settlementID string,
	providerCaptureID string,
) error {
	if db == nil {
		return errors.New("FLEX settlement database is required")
	}

	if _, err := uuid.Parse(settlementID); err != nil {
		return ErrFlexSettlementNotFound
	}

	providerCaptureID = strings.TrimSpace(providerCaptureID)
	if providerCaptureID == "" {
		return errors.New("FLEX provider capture identity is required")
	}

	return RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var paymentID, transactionID string

		// Resolve identifiers without acquiring row locks.
		err := tx.QueryRow(ctx, `
            SELECT payment_id, capture_transaction_id
            FROM flex_settlements
            WHERE id = $1
        `, settlementID).Scan(&paymentID, &transactionID)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexSettlementNotFound
		}
		if err != nil {
			return fmt.Errorf("read FLEX settlement links: %w", err)
		}

		payments := NewPaymentRepositoryWithDB(tx)
		transactions := NewPaymentTransactionRepositoryWithDB(tx)
		earnings := NewDriverEarningRepositoryWithDB(tx)

		// Preserve the established payment-before-transaction lock order.
		payment, err := payments.GetByIDForUpdate(ctx, paymentID)
		if err != nil {
			return fmt.Errorf("lock FLEX payment: %w", err)
		}

		capture, err := transactions.GetByIDForUpdate(
			ctx, transactionID,
		)
		if err != nil {
			return fmt.Errorf("lock FLEX capture transaction: %w", err)
		}

		var tripID, authorizationID, providerID, captureKey string
		var amount, currency, provider, status string
		var linkedPaymentID, linkedTransactionID string
		var existingProviderCaptureID *string

		err = tx.QueryRow(ctx, `
            SELECT
                trip_id,
                authorization_id,
                provider_authorization_id,
                capture_idempotency_key,
                amount::text,
                currency,
                provider,
                status,
                payment_id,
                capture_transaction_id,
                provider_capture_id
            FROM flex_settlements
            WHERE id = $1
            FOR UPDATE
        `, settlementID).Scan(
			&tripID,
			&authorizationID,
			&providerID,
			&captureKey,
			&amount,
			&currency,
			&provider,
			&status,
			&linkedPaymentID,
			&linkedTransactionID,
			&existingProviderCaptureID,
		)
		if err != nil {
			return fmt.Errorf("lock FLEX settlement: %w", err)
		}

		_ = authorizationID

		if linkedPaymentID != paymentID ||
			linkedTransactionID != transactionID ||
			provider != "STRIPE" ||
			providerID != providerCaptureID ||
			payment.TripID != tripID ||
			payment.ID != capture.PaymentID ||
			payment.Currency != currency ||
			!flexAmountsEqual(payment.Amount, amount) ||
			capture.Provider != provider ||
			capture.TransactionType != "CAPTURE" ||
			capture.Currency != currency ||
			!flexAmountsEqual(capture.Amount, amount) ||
			capture.IdempotencyKey == nil ||
			*capture.IdempotencyKey != captureKey {
			return errors.New(
				"FLEX capture financial or provider identity mismatch",
			)
		}

		if capture.ProviderTransactionID != nil &&
			*capture.ProviderTransactionID != providerCaptureID {
			return errors.New(
				"FLEX capture provider transaction identity conflict",
			)
		}

		// An identical replay must not modify settled financial records.
		if status == "CAPTURED" {
			if existingProviderCaptureID == nil ||
				*existingProviderCaptureID != providerCaptureID ||
				capture.Status != "SUCCESS" ||
				payment.Status != "PAID" {
				return ErrFlexSettlementInvalidState
			}
			return nil
		}

		if status != "PROCESSING" ||
			(capture.Status != "PENDING" &&
				capture.Status != "PROCESSING") ||
			(payment.Status != "PENDING" &&
				payment.Status != "PROCESSING" &&
				payment.Status != "AUTHORIZED") {
			return ErrFlexSettlementInvalidState
		}

		if err := transactions.UpdateResult(
			ctx,
			repository.UpdatePaymentTransactionResultParams{
				ID:                    capture.ID,
				Status:                "SUCCESS",
				ProviderTransactionID: &providerCaptureID,
			},
		); err != nil {
			return fmt.Errorf("finalize FLEX capture transaction: %w", err)
		}

		if err := payments.UpdateStatus(
			ctx, payment.ID, "PAID",
		); err != nil {
			return fmt.Errorf("finalize FLEX payment: %w", err)
		}

		if _, err := earnings.CreateFromPaidPayment(
			ctx, payment.ID,
		); err != nil {
			return fmt.Errorf("create FLEX driver earning: %w", err)
		}

		tag, err := tx.Exec(ctx, `
            UPDATE flex_settlements
            SET status = 'CAPTURED',
                provider_capture_id = $2,
                captured_at = clock_timestamp(),
                last_error = NULL,
                updated_at = clock_timestamp()
            WHERE id = $1
              AND status = 'PROCESSING'
        `, settlementID, providerCaptureID)

		if err != nil {
			return fmt.Errorf("finalize FLEX settlement: %w", err)
		}

		if tag.RowsAffected() != 1 {
			return ErrFlexSettlementInvalidState
		}

		return nil
	})
}
