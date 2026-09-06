package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (r *PaymentTransactionRepository) GetLatestSuccessfulByPaymentAndTypes(
	ctx context.Context,
	paymentID string,
	provider string,
	transactionTypes []string,
) (*models.PaymentTransaction, error) {
	const query = `
		SELECT
			` + paymentTransactionColumns + `
		FROM payment_transactions
		WHERE payment_id = $1
		  AND provider = $2
		  AND transaction_type = ANY($3)
		  AND status = 'SUCCESS'
		  AND provider_transaction_id IS NOT NULL
		ORDER BY processed_at DESC NULLS LAST,
		         created_at DESC
		LIMIT 1
	`

	transaction, err :=
		scanPaymentTransaction(
			r.db.QueryRow(
				ctx,
				query,
				paymentID,
				provider,
				transactionTypes,
			),
		)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get latest successful payment transaction: %w",
			err,
		)
	}

	return transaction, nil
}
