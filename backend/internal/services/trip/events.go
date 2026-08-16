package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *tripService) ListEvents(
	ctx context.Context,
	tripID string,
) ([]*models.TripEvent, error) {

	if tripID == "" {
		return nil, fmt.Errorf("trip ID is required")
	}

	// Confirm the trip exists before returning its event history.
	if _, err := s.repo.GetByID(
		ctx,
		tripID,
	); err != nil {
		return nil, fmt.Errorf(
			"get trip before listing events: %w",
			err,
		)
	}

	events, err := s.tripEvents.ListByTripID(
		ctx,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list trip events: %w",
			err,
		)
	}

	return events, nil
}
