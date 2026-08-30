package tripmeter

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Dependencies contains the repositories required by the trip meter service.
type Dependencies struct {
	TripLocations repository.TripLocationRepository
}

// Service calculates authoritative operational trip measurements.
type Service struct {
	tripLocations repository.TripLocationRepository
	config        Config
}

// NewService creates a trip meter service using the default meter rules.
func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		tripLocations: deps.TripLocations,
		config:        DefaultConfig(),
	}
}

// NewServiceWithConfig creates a trip meter service using explicit rules.
//
// This constructor is primarily useful for tests and future centrally
// managed trip-meter policy.
func NewServiceWithConfig(
	deps Dependencies,
	cfg Config,
) *Service {
	return &Service{
		tripLocations: deps.TripLocations,
		config:        cfg,
	}
}

// MeasureTrip calculates authoritative trip measurements from the
// persisted GPS/location evidence for the supplied trip.
func (s *Service) MeasureTrip(
	ctx context.Context,
	tripID string,
) (Measurement, error) {

	if s == nil {
		return Measurement{}, fmt.Errorf(
			"trip meter service is required",
		)
	}

	if s.tripLocations == nil {
		return Measurement{}, fmt.Errorf(
			"trip location repository is required",
		)
	}

	if tripID == "" {
		return Measurement{}, fmt.Errorf(
			"trip id is required",
		)
	}

	locations, err := s.tripLocations.ListByTripID(
		ctx,
		tripID,
	)
	if err != nil {
		return Measurement{}, fmt.Errorf(
			"list trip locations: %w",
			err,
		)
	}

	return Calculate(
		locations,
		s.config,
	)
}
