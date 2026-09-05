package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

var ErrRefundAmountExceedsRemaining = errors.New(
	"refund amount exceeds remaining refundable amount",
)

func (r *PaymentTransactionRepository) ValidateRefundAmount(
	ctx context.Context,
	paymentID string,
	refundAmount string,
) error {
	const query = `
		SELECT
			CASE
				WHEN $2::numeric <= 0
					THEN FALSE

				WHEN $2::numeric >
					(
						p.amount -
						COALESCE(
							SUM(pt.amount) FILTER (
								WHERE pt.transaction_type = 'REFUND'
								  AND pt.status = 'SUCCESS'
							),
							0
						)
					)
					THEN FALSE

				ELSE TRUE
			END
		FROM payments p
		LEFT JOIN payment_transactions pt
			ON pt.payment_id = p.id
		WHERE p.id = $1
		GROUP BY p.amount
	`

	var valid bool

	err := r.db.QueryRow(
		ctx,
		query,
		paymentID,
		refundAmount,
	).Scan(
		&valid,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}

		return fmt.Errorf(
			"validate refund amount: %w",
			err,
		)
	}

	if !valid {
		return repository.ErrRefundAmountExceedsRemaining
	}

	return nil
}
