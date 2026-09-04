package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *PaymentTransactionRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.PaymentTransaction, error) {
	query := `
		SELECT
			` + paymentTransactionColumns + `
		FROM payment_transactions
		WHERE id = $1
	`

	transaction, err := scanPaymentTransaction(
		r.db.QueryRow(ctx, query, id),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get payment transaction by id: %w",
			err,
		)
	}

	return transaction, nil
}

func (r *PaymentTransactionRepository) GetByReference(
	ctx context.Context,
	transactionReference string,
) (*models.PaymentTransaction, error) {
	query := `
		SELECT
			` + paymentTransactionColumns + `
		FROM payment_transactions
		WHERE transaction_reference = $1
	`

	transaction, err := scanPaymentTransaction(
		r.db.QueryRow(
			ctx,
			query,
			transactionReference,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get payment transaction by reference: %w",
			err,
		)
	}

	return transaction, nil
}

func (r *PaymentTransactionRepository) GetByProviderIdempotencyKey(
	ctx context.Context,
	provider string,
	idempotencyKey string,
) (*models.PaymentTransaction, error) {
	query := `
		SELECT
			` + paymentTransactionColumns + `
		FROM payment_transactions
		WHERE provider = $1
		  AND idempotency_key = $2
	`

	transaction, err := scanPaymentTransaction(
		r.db.QueryRow(
			ctx,
			query,
			provider,
			idempotencyKey,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get payment transaction by provider idempotency key: %w",
			err,
		)
	}

	return transaction, nil
}

func (r *PaymentTransactionRepository) GetByProviderTransactionID(
	ctx context.Context,
	provider string,
	providerTransactionID string,
) (*models.PaymentTransaction, error) {
	query := `
		SELECT
			` + paymentTransactionColumns + `
		FROM payment_transactions
		WHERE provider = $1
		  AND provider_transaction_id = $2
	`

	transaction, err := scanPaymentTransaction(
		r.db.QueryRow(
			ctx,
			query,
			provider,
			providerTransactionID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get payment transaction by provider transaction id: %w",
			err,
		)
	}

	return transaction, nil
}
