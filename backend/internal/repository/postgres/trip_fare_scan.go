package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/models"
)

type tripFareScanner interface {
	Scan(dest ...any) error
}

func scanTripFare(
	scanner tripFareScanner,
) (*models.TripFare, error) {
	var fare models.TripFare

	err := scanner.Scan(
		&fare.ID,
		&fare.TripID,
		&fare.BaseFare,
		&fare.DistanceFare,
		&fare.TimeFare,
		&fare.WaitingFare,
		&fare.BookingFee,
		&fare.SurgeMultiplier,
		&fare.SurgeAmount,
		&fare.DiscountAmount,
		&fare.TaxAmount,
		&fare.TollAmount,
		&fare.ParkingAmount,
		&fare.TotalAmount,
		&fare.Currency,
		&fare.DistanceRatePerKM,
		&fare.TimeRatePerMinute,
		&fare.WaitingRatePerMinute,
		&fare.ChargedDistanceMeters,
		&fare.ChargedDurationSeconds,
		&fare.WaitingDurationSeconds,
		&fare.PricingVersion,
		&fare.CalculatedAt,
		&fare.CreatedAt,
		&fare.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &fare, nil
}
