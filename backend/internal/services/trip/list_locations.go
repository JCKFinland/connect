package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *tripService) ListTripLocations(
	ctx context.Context,
	tripID string,
	userID string,
) ([]*models.TripLocation, error) {
	if tripID == "" {
		return nil, fmt.Errorf("trip ID is required")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Reuse the canonical trip read-authorization boundary.
	//
	// This permits:
	//   - SYSTEM_ADMIN
	//   - COMPANY_ADMIN
	//   - DISPATCHER
	//   - the assigned driver
	//   - the owning customer
	//
	// and prevents unrelated authenticated users from accessing
	// persisted trip GPS evidence.
	if _, err := s.GetByIDAuthorized(
		ctx,
		tripID,
		userID,
	); err != nil {
		return nil, err
	}

	if s.tripLocations == nil {
		return nil, fmt.Errorf(
			"trip location repository is not configured",
		)
	}

	locations, err := s.tripLocations.ListByTripID(
		ctx,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list trip locations: %w",
			err,
		)
	}

	if locations == nil {
		locations = make([]*models.TripLocation, 0)
	}

	return locations, nil
}
