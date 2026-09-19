package fare

import (
	"math"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *fareService) Calculate(
	input CalculationInput,
) (*models.TripFare, error) {
	if input.TripID == "" {
		return nil, ErrInvalidTripID
	}

	result, err := s.Estimate(
		EstimateInput{
			DistanceMeters:  input.DistanceMeters,
			DurationSeconds: input.DurationSeconds,
			WaitingSeconds:  input.WaitingSeconds,
			Pricing:         input.Pricing,
		},
	)
	if err != nil {
		return nil, err
	}

	return &models.TripFare{
		TripID: input.TripID,

		BaseFare:     result.BaseFare,
		DistanceFare: result.DistanceFare,
		TimeFare:     result.TimeFare,
		WaitingFare:  result.WaitingFare,
		BookingFee:   result.BookingFee,

		SurgeMultiplier: result.SurgeMultiplier,
		SurgeAmount:     result.SurgeAmount,

		DiscountAmount: result.DiscountAmount,
		TaxAmount:      result.TaxAmount,
		TollAmount:     result.TollAmount,
		ParkingAmount:  result.ParkingAmount,

		TotalAmount: result.TotalAmount,
		Currency:    result.Currency,

		DistanceRatePerKM: result.DistanceRatePerKM,

		TimeRatePerMinute: result.TimeRatePerMinute,

		WaitingRatePerMinute: result.WaitingRatePerMinute,

		ChargedDistanceMeters: result.DistanceMeters,

		ChargedDurationSeconds: result.DurationSeconds,

		WaitingDurationSeconds: result.WaitingSeconds,

		PricingVersion: result.PricingVersion,

		CalculatedAt: time.Now().UTC(),
	}, nil
}

func roundCurrency(
	value float64,
) float64 {
	return math.Round(
		value*100,
	) / 100
}
