package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// CreateFromPaidPayment creates the driver's earning directly from
// authoritative payment, trip, and fare records.
//
// Monetary and ownership values are derived by PostgreSQL. The caller
// supplies only the payment ID.
func (r *DriverEarningRepository) CreateFromPaidPayment(
	ctx context.Context,
	paymentID string,
) (*models.DriverEarning, error) {
	const query = `
		INSERT INTO driver_earnings (
			trip_id,
			driver_id,
			company_id,
			fare_id,
			payment_id,
			gross_amount,
			commission_amount,
			bonus_amount,
			tip_amount,
			adjustment_amount,
			tax_withheld,
			net_amount,
			currency,
			settlement_status
		)
		SELECT
			t.id,
			t.driver_id,
			t.company_id,
			tf.id,
			p.id,
			p.amount,
			0,
			0,
			0,
			0,
			0,
			p.amount,
			p.currency,
			'PENDING'
		FROM payments p
		INNER JOIN trips t
			ON t.id = p.trip_id
		INNER JOIN trip_fares tf
			ON tf.id = p.fare_id
			AND tf.trip_id = t.id
		WHERE p.id = $1
		  AND p.status = 'PAID'
		  AND t.status = 'COMPLETED'
		  AND t.deleted_at IS NULL
		ON CONFLICT (trip_id) DO UPDATE
		SET updated_at = driver_earnings.updated_at
		RETURNING
			` + driverEarningColumns

	earning, err := scanDriverEarning(
		r.db.QueryRow(
			ctx,
			query,
			paymentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"create driver earning from paid payment: %w",
			err,
		)
	}

	return earning, nil
}
