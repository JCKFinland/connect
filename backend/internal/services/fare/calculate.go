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

	if input.DistanceMeters < 0 {
		return nil, ErrInvalidDistance
	}

	if input.DurationSeconds < 0 {
		return nil, ErrInvalidDuration
	}

	if input.WaitingSeconds < 0 {
		return nil, ErrInvalidWaitingDuration
	}

	pricing := input.Pricing

	if pricing.BaseFare < 0 ||
		pricing.DistanceRatePerKM < 0 ||
		pricing.TimeRatePerMinute < 0 ||
		pricing.WaitingRatePerMinute < 0 ||
		pricing.BookingFee < 0 ||
		pricing.DiscountAmount < 0 ||
		pricing.TaxAmount < 0 ||
		pricing.TollAmount < 0 ||
		pricing.ParkingAmount < 0 {
		return nil, ErrInvalidPricing
	}

	if pricing.SurgeMultiplier < 1 {
		return nil, ErrInvalidSurgeMultiplier
	}

	if pricing.Currency == "" {
		return nil, ErrInvalidCurrency
	}

	if pricing.PricingVersion == "" {
		return nil, ErrInvalidPricingVersion
	}

	distanceKM :=
		float64(input.DistanceMeters) / 1000

	durationMinutes :=
		float64(input.DurationSeconds) / 60

	waitingMinutes :=
		float64(input.WaitingSeconds) / 60

	baseFare := roundCurrency(
		pricing.BaseFare,
	)

	distanceFare := roundCurrency(
		distanceKM *
			pricing.DistanceRatePerKM,
	)

	timeFare := roundCurrency(
		durationMinutes *
			pricing.TimeRatePerMinute,
	)

	waitingFare := roundCurrency(
		waitingMinutes *
			pricing.WaitingRatePerMinute,
	)

	bookingFee := roundCurrency(
		pricing.BookingFee,
	)

	preSurgeSubtotal :=
		baseFare +
			distanceFare +
			timeFare +
			waitingFare +
			bookingFee

	surgeAmount := roundCurrency(
		preSurgeSubtotal *
			(pricing.SurgeMultiplier - 1),
	)

	discountAmount := roundCurrency(
		pricing.DiscountAmount,
	)

	taxAmount := roundCurrency(
		pricing.TaxAmount,
	)

	tollAmount := roundCurrency(
		pricing.TollAmount,
	)

	parkingAmount := roundCurrency(
		pricing.ParkingAmount,
	)

	totalAmount := roundCurrency(
		preSurgeSubtotal +
			surgeAmount +
			taxAmount +
			tollAmount +
			parkingAmount -
			discountAmount,
	)

	// Never allow a negative payable fare.
	if totalAmount < 0 {
		totalAmount = 0
	}

	now := time.Now().UTC()

	return &models.TripFare{
		TripID: input.TripID,

		BaseFare:     baseFare,
		DistanceFare: distanceFare,
		TimeFare:     timeFare,
		WaitingFare:  waitingFare,
		BookingFee:   bookingFee,

		SurgeMultiplier: pricing.SurgeMultiplier,
		SurgeAmount:     surgeAmount,

		DiscountAmount: discountAmount,
		TaxAmount:      taxAmount,
		TollAmount:     tollAmount,
		ParkingAmount:  parkingAmount,

		TotalAmount: totalAmount,
		Currency:    pricing.Currency,

		DistanceRatePerKM: pricing.DistanceRatePerKM,

		TimeRatePerMinute: pricing.TimeRatePerMinute,

		WaitingRatePerMinute: pricing.WaitingRatePerMinute,

		ChargedDistanceMeters: input.DistanceMeters,

		ChargedDurationSeconds: input.DurationSeconds,

		WaitingDurationSeconds: input.WaitingSeconds,

		PricingVersion: pricing.PricingVersion,

		CalculatedAt: now,
	}, nil
}

func roundCurrency(
	value float64,
) float64 {
	return math.Round(
		value*100,
	) / 100
}
