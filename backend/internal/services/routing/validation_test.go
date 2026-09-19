package routing

import (
	"errors"
	"testing"
)

func TestValidateRequestAcceptsValidCoordinates(t *testing.T) {
	request := RouteRequest{
		Origin: Coordinate{
			Latitude:  60.2055,
			Longitude: 24.6559,
		},
		Destination: Coordinate{
			Latitude:  60.1719,
			Longitude: 24.9414,
		},
	}

	if err := validateRequest(request); err != nil {
		t.Fatalf("validateRequest() unexpected error: %v", err)
	}
}

func TestValidateRequestRejectsInvalidOrigin(t *testing.T) {
	request := RouteRequest{
		Origin: Coordinate{
			Latitude: 91,
		},
		Destination: Coordinate{
			Latitude:  60.1719,
			Longitude: 24.9414,
		},
	}

	err := validateRequest(request)

	if !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf(
			"validateRequest() error = %v, want %v",
			err,
			ErrInvalidOrigin,
		)
	}
}

func TestValidateRequestRejectsInvalidDestination(t *testing.T) {
	request := RouteRequest{
		Origin: Coordinate{
			Latitude:  60.2055,
			Longitude: 24.6559,
		},
		Destination: Coordinate{
			Longitude: 181,
		},
	}

	err := validateRequest(request)

	if !errors.Is(err, ErrInvalidDestination) {
		t.Fatalf(
			"validateRequest() error = %v, want %v",
			err,
			ErrInvalidDestination,
		)
	}
}
