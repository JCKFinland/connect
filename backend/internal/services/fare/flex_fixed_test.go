package fare

import (
	"testing"
	"time"
)

func validFlexFixedFareInput() FlexFixedFareInput {
	return FlexFixedFareInput{
		TripID:           "trip-123",
		AmountCents:      2328,
		Currency:         "EUR",
		PricingProfileID: "profile-123",
		PricingVersion:   "pricing-v1",
		CalculatedAt: time.Date(
			2026, 10, 17, 12, 0, 0, 0, time.UTC,
		),
	}
}

func TestBuildFlexFixedFare(t *testing.T) {
	input := validFlexFixedFareInput()

	got, err := BuildFlexFixedFare(input)
	if err != nil {
		t.Fatal(err)
	}

	if got.TripID != input.TripID {
		t.Fatalf("unexpected trip ID: %s", got.TripID)
	}

	if got.TotalAmount != 23.28 ||
		got.BaseFare != 23.28 {
		t.Fatalf(
			"unexpected fixed fare: total=%v base=%v",
			got.TotalAmount,
			got.BaseFare,
		)
	}

	if got.Currency != "EUR" ||
		got.SurgeMultiplier != 1 {
		t.Fatal("invalid fixed fare currency or multiplier")
	}

	if got.DistanceFare != 0 ||
		got.TimeFare != 0 ||
		got.WaitingFare != 0 ||
		got.BookingFee != 0 ||
		got.SurgeAmount != 0 ||
		got.DiscountAmount != 0 ||
		got.TaxAmount != 0 ||
		got.TollAmount != 0 ||
		got.ParkingAmount != 0 {
		t.Fatal("fixed fare contains unexpected components")
	}

	if got.ChargedDistanceMeters != 0 ||
		got.ChargedDurationSeconds != 0 ||
		got.WaitingDurationSeconds != 0 {
		t.Fatal("fixed fare must not invent billable meter values")
	}

	if got.PricingProfileID == nil ||
		*got.PricingProfileID != input.PricingProfileID ||
		got.PricingVersion != input.PricingVersion {
		t.Fatal("FLEX pricing provenance was not preserved")
	}

	if !got.CalculatedAt.Equal(input.CalculatedAt) {
		t.Fatal("calculation timestamp mismatch")
	}
}

func TestBuildFlexFixedFareRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		change func(*FlexFixedFareInput)
	}{
		{
			name: "missing trip ID",
			change: func(v *FlexFixedFareInput) {
				v.TripID = ""
			},
		},
		{
			name: "zero amount",
			change: func(v *FlexFixedFareInput) {
				v.AmountCents = 0
			},
		},
		{
			name: "negative amount",
			change: func(v *FlexFixedFareInput) {
				v.AmountCents = -1
			},
		},
		{
			name: "amount exceeds database limit",
			change: func(v *FlexFixedFareInput) {
				v.AmountCents = maxFlexFixedFareCents + 1
			},
		},
		{
			name: "unsupported currency",
			change: func(v *FlexFixedFareInput) {
				v.Currency = "USD"
			},
		},
		{
			name: "missing pricing profile",
			change: func(v *FlexFixedFareInput) {
				v.PricingProfileID = ""
			},
		},
		{
			name: "missing pricing version",
			change: func(v *FlexFixedFareInput) {
				v.PricingVersion = ""
			},
		},
		{
			name: "missing timestamp",
			change: func(v *FlexFixedFareInput) {
				v.CalculatedAt = time.Time{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validFlexFixedFareInput()
			tt.change(&input)

			if _, err := BuildFlexFixedFare(input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
