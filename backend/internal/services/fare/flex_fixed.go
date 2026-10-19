package fare

import (
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

const maxFlexFixedFareCents int64 = 9_999_999_999

// FlexFixedFareInput contains the authoritative negotiated price
// and immutable auction pricing provenance.
//
// AmountCents must originate from an authorized FLEX payment,
// never directly from a passenger or driver request.
type FlexFixedFareInput struct {
	TripID           string
	AmountCents      int64
	Currency         string
	PricingProfileID string
	PricingVersion   string
	CalculatedAt     time.Time
}

// BuildFlexFixedFare constructs a fixed contractual fare.
//
// Actual GPS measurements are persisted separately in the
// authoritative trip meter measurement record.
//
// This function does not verify authorization or ownership.
// Those checks belong to the transaction-bound caller.
func BuildFlexFixedFare(
	input FlexFixedFareInput,
) (*models.TripFare, error) {
	if input.TripID == "" {
		return nil, errors.New("FLEX trip ID is required")
	}

	if input.AmountCents <= 0 ||
		input.AmountCents > maxFlexFixedFareCents {
		return nil, fmt.Errorf(
			"invalid FLEX fixed fare amount: %d cents",
			input.AmountCents,
		)
	}

	if input.Currency != "EUR" {
		return nil, errors.New("FLEX currency must be EUR")
	}

	if input.PricingProfileID == "" {
		return nil, errors.New(
			"FLEX pricing profile ID is required",
		)
	}

	if input.PricingVersion == "" {
		return nil, errors.New(
			"FLEX pricing version is required",
		)
	}

	if input.CalculatedAt.IsZero() {
		return nil, errors.New(
			"FLEX calculation timestamp is required",
		)
	}

	amount := float64(input.AmountCents) / 100

	profileID := input.PricingProfileID

	return &models.TripFare{
		TripID: input.TripID,

		BaseFare:     amount,
		DistanceFare: 0,
		TimeFare:     0,
		WaitingFare:  0,
		BookingFee:   0,

		SurgeMultiplier: 1,
		SurgeAmount:     0,

		DiscountAmount: 0,
		TaxAmount:      0,
		TollAmount:     0,
		ParkingAmount:  0,

		TotalAmount: amount,
		Currency:    input.Currency,

		DistanceRatePerKM:    0,
		TimeRatePerMinute:    0,
		WaitingRatePerMinute: 0,

		ChargedDistanceMeters:  0,
		ChargedDurationSeconds: 0,
		WaitingDurationSeconds: 0,

		PricingVersion:   input.PricingVersion,
		PricingProfileID: &profileID,
		CalculatedAt:     input.CalculatedAt.UTC(),
	}, nil
}
