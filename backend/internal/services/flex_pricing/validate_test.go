package flex_pricing

import (
	"errors"
	"math"
	"testing"
)

func TestBoundsFromEstimate(t *testing.T) {
	bounds, err := BoundsFromEstimate(23.28, "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bounds.BaselineCents != 2328 ||
		bounds.MinimumCents != 2095 ||
		bounds.MaximumCents != 2561 {
		t.Fatalf("unexpected bounds: %+v", bounds)
	}
}

func TestBoundsFromEstimateRejectsInvalidBaseline(t *testing.T) {
	tests := []struct {
		name     string
		total    float64
		currency string
	}{
		{"zero", 0, "EUR"},
		{"negative", -1, "EUR"},
		{"nan", math.NaN(), "EUR"},
		{"positive infinity", math.Inf(1), "EUR"},
		{"negative infinity", math.Inf(-1), "EUR"},
		{"wrong currency", 10, "USD"},
		{"too large", 100000000, "EUR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BoundsFromEstimate(tt.total, tt.currency)
			if !errors.Is(err, ErrInvalidBaseline) {
				t.Fatalf("expected invalid baseline, got %v", err)
			}
		})
	}
}

func TestValidateOffer(t *testing.T) {
	bounds, err := BoundsFromEstimate(23.28, "EUR")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		offer   int64
		wantErr error
	}{
		{"minimum", 2095, nil},
		{"baseline", 2328, nil},
		{"maximum", 2561, nil},
		{"below minimum", 2094, ErrOfferOutOfRange},
		{"above maximum", 2562, ErrOfferOutOfRange},
		{"zero", 0, ErrInvalidOffer},
		{"negative", -1, ErrInvalidOffer},
		{"overflow", 10_000_000_000, ErrInvalidOffer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOffer(tt.offer, bounds)

			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestBoundsFromEstimateSmallAmounts(t *testing.T) {
	tests := []struct {
		name     string
		total    float64
		baseline int64
		minimum  int64
		maximum  int64
	}{
		{"one cent", 0.01, 1, 1, 2},
		{"two cents", 0.02, 2, 1, 3},
		{"ten cents", 0.10, 10, 9, 11},
		{"one euro", 1.00, 100, 90, 110},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BoundsFromEstimate(tt.total, "EUR")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.BaselineCents != tt.baseline ||
				got.MinimumCents != tt.minimum ||
				got.MaximumCents != tt.maximum {
				t.Fatalf(
					"got %+v, want baseline=%d minimum=%d maximum=%d",
					got,
					tt.baseline,
					tt.minimum,
					tt.maximum,
				)
			}
		})
	}
}

func TestBoundsFromEstimateMaximumAmount(t *testing.T) {
	bounds, err := BoundsFromEstimate(99_999_999.99, "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bounds.BaselineCents != 9_999_999_999 {
		t.Fatalf("unexpected baseline: %+v", bounds)
	}

	if bounds.MaximumCents != 9_999_999_999 {
		t.Fatalf("maximum was not capped: %+v", bounds)
	}

	if err := ValidateOffer(bounds.MaximumCents, bounds); err != nil {
		t.Fatalf("maximum permitted offer rejected: %v", err)
	}
}

func TestValidateOfferRejectsMalformedBounds(t *testing.T) {
	tests := []struct {
		name   string
		bounds BidBounds
	}{
		{"empty", BidBounds{}},
		{"zero baseline", BidBounds{
			MinimumCents: 100,
			MaximumCents: 200,
		}},
		{"zero minimum", BidBounds{
			BaselineCents: 150,
			MaximumCents:  200,
		}},
		{"reversed range", BidBounds{
			BaselineCents: 150,
			MinimumCents:  200,
			MaximumCents:  100,
		}},
		{"maximum overflow", BidBounds{
			BaselineCents: 150,
			MinimumCents:  100,
			MaximumCents:  10_000_000_000,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOffer(150, tt.bounds)

			if !errors.Is(err, ErrInvalidBaseline) {
				t.Fatalf("expected invalid baseline, got %v", err)
			}
		})
	}
}
