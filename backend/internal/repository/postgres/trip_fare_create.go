package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *TripFareRepository) Create(
	ctx context.Context,
	fare *models.TripFare,
) error {
	const query = `
		INSERT INTO trip_fares (
			trip_id,
			base_fare,
			distance_fare,
			time_fare,
			waiting_fare,
			booking_fee,
			surge_multiplier,
			surge_amount,
			discount_amount,
			tax_amount,
			toll_amount,
			parking_amount,
			total_amount,
			currency,
			distance_rate_per_km,
			time_rate_per_minute,
			waiting_rate_per_minute,
			charged_distance_meters,
			charged_duration_seconds,
			waiting_duration_seconds,
			pricing_version,
			calculated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22
		)
		RETURNING
			id,
			created_at,
			updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		fare.TripID,
		fare.BaseFare,
		fare.DistanceFare,
		fare.TimeFare,
		fare.WaitingFare,
		fare.BookingFee,
		fare.SurgeMultiplier,
		fare.SurgeAmount,
		fare.DiscountAmount,
		fare.TaxAmount,
		fare.TollAmount,
		fare.ParkingAmount,
		fare.TotalAmount,
		fare.Currency,
		fare.DistanceRatePerKM,
		fare.TimeRatePerMinute,
		fare.WaitingRatePerMinute,
		fare.ChargedDistanceMeters,
		fare.ChargedDurationSeconds,
		fare.WaitingDurationSeconds,
		fare.PricingVersion,
		fare.CalculatedAt,
	).Scan(
		&fare.ID,
		&fare.CreatedAt,
		&fare.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create trip fare: %w", err)
	}

	return nil
}
