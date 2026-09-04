package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *PaymentTransactionRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.PaymentTransaction, error) {
	query := `
		SELECT
			` + paymentTransactionColumns + `
		FROM payment_transactions
		WHERE id = $1
		FOR UPDATE
	`

	transaction, err := scanPaymentTransaction(
		r.db.QueryRow(
			ctx,
			query,
			id,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get payment transaction by id for update: %w",
			err,
		)
	}

	return transaction, nil
}
