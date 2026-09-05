package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *PaymentTransactionRepository) GetSuccessfulRefundState(
	ctx context.Context,
	paymentID string,
) (repository.SuccessfulRefundState, error) {
	const query = `
		SELECT
			CASE
				WHEN COALESCE(
					SUM(pt.amount) FILTER (
						WHERE pt.transaction_type = 'REFUND'
						  AND pt.status = 'SUCCESS'
					),
					0
				) > p.amount
					THEN 'OVER'

				WHEN COALESCE(
					SUM(pt.amount) FILTER (
						WHERE pt.transaction_type = 'REFUND'
						  AND pt.status = 'SUCCESS'
					),
					0
				) = p.amount
				AND p.amount > 0
					THEN 'FULL'

				WHEN COALESCE(
					SUM(pt.amount) FILTER (
						WHERE pt.transaction_type = 'REFUND'
						  AND pt.status = 'SUCCESS'
					),
					0
				) > 0
					THEN 'PARTIAL'

				ELSE 'NONE'
			END
		FROM payments p
		LEFT JOIN payment_transactions pt
			ON pt.payment_id = p.id
		WHERE p.id = $1
		GROUP BY p.amount
	`

	var state repository.SuccessfulRefundState

	err := r.db.QueryRow(
		ctx,
		query,
		paymentID,
	).Scan(
		&state,
	)

	if err == pgx.ErrNoRows {
		return "", pgx.ErrNoRows
	}

	if err != nil {
		return "", fmt.Errorf(
			"get successful refund state: %w",
			err,
		)
	}

	return state, nil
}