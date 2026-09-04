package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (r *PaymentTransactionRepository) Create(
	ctx context.Context,
	params repository.CreatePaymentTransactionParams,
) (*models.PaymentTransaction, error) {
	const query = `
		INSERT INTO payment_transactions (
			payment_id,
			transaction_reference,
			provider,
			idempotency_key,
			transaction_type,
			status,
			amount,
			currency,
			gateway_request
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			'PENDING',
			$6::numeric,
			$7,
			$8
		)
		RETURNING
			` + paymentTransactionColumns

	transaction, err := scanPaymentTransaction(
		r.db.QueryRow(
			ctx,
			query,
			params.PaymentID,
			params.TransactionReference,
			params.Provider,
			params.IdempotencyKey,
			params.TransactionType,
			params.Amount,
			params.Currency,
			params.GatewayRequest,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create payment transaction: %w",
			err,
		)
	}

	return transaction, nil
}
