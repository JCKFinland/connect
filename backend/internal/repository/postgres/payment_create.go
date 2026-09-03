package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

// CreateFromCompletedTrip creates a payment directly from the
// authoritative fare belonging to a completed trip.
//
// The caller supplies only:
//   - trip ID
//   - payment method
//
// PostgreSQL derives:
//   - fare ID
//   - customer ID
//   - amount
//   - currency
//
// This prevents client or application code from constructing a payment
// whose monetary identity differs from the authoritative trip fare.
func (r *PaymentRepository) CreateFromCompletedTrip(
	ctx context.Context,
	tripID string,
	paymentMethod string,
) (*models.Payment, error) {
	const query = `
		INSERT INTO payments (
			trip_id,
			fare_id,
			customer_id,
			status,
			payment_method,
			amount,
			currency
		)
		SELECT
			t.id,
			tf.id,
			t.customer_id,
			'PENDING',
			$2,
			tf.total_amount,
			tf.currency
		FROM trips t
		INNER JOIN trip_fares tf
			ON tf.trip_id = t.id
		WHERE t.id = $1
		  AND t.status = 'COMPLETED'
		  AND t.deleted_at IS NULL
		RETURNING
			` + paymentColumns

	payment, err := scanPayment(
		r.db.QueryRow(
			ctx,
			query,
			tripID,
			paymentMethod,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"create payment from completed trip: %w",
			err,
		)
	}

	return payment, nil
}
