package dispatch

import (
	"math"
	"testing"
)

func TestDistanceKM_SameLocation(t *testing.T) {
	got := distanceKM(
		60.2055,
		24.6559,
		60.2055,
		24.6559,
	)

	if got != 0 {
		t.Fatalf(
			"expected 0 km, got %f",
			got,
		)
	}
}

func TestDistanceKM_EspooToHelsinki(t *testing.T) {
	got := distanceKM(
		60.2055,
		24.6559,
		60.1699,
		24.9384,
	)

	// Straight-line distance should be roughly 16 km.
	if math.Abs(got-16.0) > 2.0 {
		t.Fatalf(
			"expected about 16 km, got %f",
			got,
		)
	}
}
