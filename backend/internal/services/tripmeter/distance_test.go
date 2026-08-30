package tripmeter

import (
	"math"
	"testing"
)

func TestHaversineDistanceMetersSamePoint(t *testing.T) {
	distance := haversineDistanceMeters(
		60.169856,
		24.938379,
		60.169856,
		24.938379,
	)

	if distance != 0 {
		t.Fatalf(
			"expected zero distance, got %f",
			distance,
		)
	}
}

func TestHaversineDistanceMetersKnownDistance(t *testing.T) {
	// Two nearby Helsinki coordinates.
	distance := haversineDistanceMeters(
		60.169856,
		24.938379,
		60.170500,
		24.940000,
	)

	// We deliberately use a range instead of asserting an exact
	// floating-point value.
	if distance < 110 || distance > 120 {
		t.Fatalf(
			"expected distance between 110 and 120 meters, got %f",
			distance,
		)
	}
}

func TestHaversineDistanceMetersSymmetric(t *testing.T) {
	forward := haversineDistanceMeters(
		60.169856,
		24.938379,
		60.170500,
		24.940000,
	)

	reverse := haversineDistanceMeters(
		60.170500,
		24.940000,
		60.169856,
		24.938379,
	)

	if math.Abs(forward-reverse) > 0.000001 {
		t.Fatalf(
			"expected symmetric distance, forward=%f reverse=%f",
			forward,
			reverse,
		)
	}
}
