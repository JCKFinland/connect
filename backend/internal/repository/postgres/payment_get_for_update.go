package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *PaymentRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.Payment, error) {
	query := `
		SELECT
			` + paymentColumns + `
		FROM payments
		WHERE id = $1
		FOR UPDATE
	`

	payment, err := scanPayment(
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
			"get payment by id for update: %w",
			err,
		)
	}

	return payment, nil
}
