package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *PaymentRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {
	const query = `
		UPDATE payments
		SET
			status = $1::varchar,

			paid_at = CASE
				WHEN $1::varchar = 'PAID'
					THEN COALESCE(paid_at, NOW())
				ELSE paid_at
			END,

			updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.db.Exec(
		ctx,
		query,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf(
			"update payment status: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
