package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *PaymentTransactionRepository) UpdateResult(
	ctx context.Context,
	params repository.UpdatePaymentTransactionResultParams,
) error {
	const query = `
		UPDATE payment_transactions
		SET
			status = $1::varchar,

			provider_transaction_id =
				COALESCE(
					$2::varchar,
					provider_transaction_id
				),

			gateway_response =
				COALESCE(
					$3::jsonb,
					gateway_response
				),

			processed_at = CASE
				WHEN $1::varchar IN (
					'SUCCESS',
					'FAILED',
					'CANCELLED'
				)
					THEN COALESCE(
						processed_at,
						NOW()
					)
				ELSE processed_at
			END,

			updated_at = NOW()

		WHERE id = $4
	`

	result, err := r.db.Exec(
		ctx,
		query,
		params.Status,
		params.ProviderTransactionID,
		params.GatewayResponse,
		params.ID,
	)
	if err != nil {
		return fmt.Errorf(
			"update payment transaction result: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
