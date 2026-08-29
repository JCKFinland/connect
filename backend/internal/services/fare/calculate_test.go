package fare

import (
	"errors"
	"testing"
)

func TestCalculateFare(t *testing.T) {
	service := NewService()

	fare, err := service.Calculate(
		CalculationInput{
			TripID: "trip-test-1",

			DistanceMeters:  8420,
			DurationSeconds: 900,
			WaitingSeconds:  150,

			Pricing: PricingSnapshot{
				BaseFare:             4.90,
				DistanceRatePerKM:    1.50,
				TimeRatePerMinute:    0.25,
				WaitingRatePerMinute: 0.50,
				BookingFee:           2.00,
				SurgeMultiplier:      1.00,
				DiscountAmount:       0,
				TaxAmount:            5.17,
				TollAmount:           0,
				ParkingAmount:        0,
				Currency:             "EUR",
				PricingVersion:       "v1",
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"calculate fare: %v",
			err,
		)
	}

	if fare.BaseFare != 4.90 {
		t.Fatalf(
			"expected base fare 4.90, got %.2f",
			fare.BaseFare,
		)
	}

	if fare.DistanceFare != 12.63 {
		t.Fatalf(
			"expected distance fare 12.63, got %.2f",
			fare.DistanceFare,
		)
	}

	if fare.TimeFare != 3.75 {
		t.Fatalf(
			"expected time fare 3.75, got %.2f",
			fare.TimeFare,
		)
	}

	if fare.WaitingFare != 1.25 {
		t.Fatalf(
			"expected waiting fare 1.25, got %.2f",
			fare.WaitingFare,
		)
	}

	if fare.TotalAmount != 29.70 {
		t.Fatalf(
			"expected total amount 29.70, got %.2f",
			fare.TotalAmount,
		)
	}

	if fare.ChargedDistanceMeters != 8420 {
		t.Fatalf(
			"expected charged distance 8420, got %d",
			fare.ChargedDistanceMeters,
		)
	}

	if fare.ChargedDurationSeconds != 900 {
		t.Fatalf(
			"expected charged duration 900, got %d",
			fare.ChargedDurationSeconds,
		)
	}

	if fare.WaitingDurationSeconds != 150 {
		t.Fatalf(
			"expected waiting duration 150, got %d",
			fare.WaitingDurationSeconds,
		)
	}

	if fare.Currency != "EUR" {
		t.Fatalf(
			"expected EUR, got %s",
			fare.Currency,
		)
	}

	if fare.PricingVersion != "v1" {
		t.Fatalf(
			"expected pricing version v1, got %s",
			fare.PricingVersion,
		)
	}

	if fare.CalculatedAt.IsZero() {
		t.Fatal(
			"expected calculated_at",
		)
	}
}

func TestCalculateFareRejectsInvalidInput(
	t *testing.T,
) {
	service := NewService()

	tests := []struct {
		name        string
		input       CalculationInput
		expectedErr error
	}{
		{
			name: "missing trip ID",
			input: CalculationInput{
				Pricing: validPricing(),
			},
			expectedErr: ErrInvalidTripID,
		},
		{
			name: "negative distance",
			input: CalculationInput{
				TripID:         "trip-1",
				DistanceMeters: -1,
				Pricing:        validPricing(),
			},
			expectedErr: ErrInvalidDistance,
		},
		{
			name: "negative duration",
			input: CalculationInput{
				TripID:          "trip-1",
				DurationSeconds: -1,
				Pricing:         validPricing(),
			},
			expectedErr: ErrInvalidDuration,
		},
		{
			name: "negative waiting duration",
			input: CalculationInput{
				TripID:         "trip-1",
				WaitingSeconds: -1,
				Pricing:        validPricing(),
			},
			expectedErr: ErrInvalidWaitingDuration,
		},
		{
			name: "invalid surge multiplier",
			input: CalculationInput{
				TripID: "trip-1",
				Pricing: PricingSnapshot{
					SurgeMultiplier: 0.99,
					Currency:        "EUR",
					PricingVersion:  "v1",
				},
			},
			expectedErr: ErrInvalidSurgeMultiplier,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := service.Calculate(
					tt.input,
				)

				if !errors.Is(
					err,
					tt.expectedErr,
				) {
					t.Fatalf(
						"expected %v, got %v",
						tt.expectedErr,
						err,
					)
				}
			},
		)
	}
}

func validPricing() PricingSnapshot {
	return PricingSnapshot{
		BaseFare:             4.90,
		DistanceRatePerKM:    1.50,
		TimeRatePerMinute:    0.25,
		WaitingRatePerMinute: 0.50,
		BookingFee:           2.00,
		SurgeMultiplier:      1.00,
		Currency:             "EUR",
		PricingVersion:       "v1",
	}
}
