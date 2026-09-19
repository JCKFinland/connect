package fare

import "testing"

func TestEstimateFareMatchesCanonicalCalculation(t *testing.T) {
	service := NewService()

	pricing := PricingSnapshot{
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
	}

	estimate, err := service.Estimate(
		EstimateInput{
			DistanceMeters:  8420,
			DurationSeconds: 900,
			WaitingSeconds:  150,
			Pricing:         pricing,
		},
	)
	if err != nil {
		t.Fatalf("Estimate() unexpected error: %v", err)
	}

	if estimate.TotalAmount != 29.70 {
		t.Fatalf(
			"total = %.2f, want 29.70",
			estimate.TotalAmount,
		)
	}

	if estimate.DistanceFare != 12.63 {
		t.Fatalf(
			"distance fare = %.2f, want 12.63",
			estimate.DistanceFare,
		)
	}

	if estimate.TimeFare != 3.75 {
		t.Fatalf(
			"time fare = %.2f, want 3.75",
			estimate.TimeFare,
		)
	}

	if estimate.WaitingFare != 1.25 {
		t.Fatalf(
			"waiting fare = %.2f, want 1.25",
			estimate.WaitingFare,
		)
	}

	if estimate.DistanceMeters != 8420 ||
		estimate.DurationSeconds != 900 ||
		estimate.WaitingSeconds != 150 {
		t.Fatalf(
			"unexpected estimate metrics: %+v",
			estimate,
		)
	}
}
