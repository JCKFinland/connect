package trip

import (
	"context"
	"fmt"
)

// AssignDriver assigns a driver and vehicle to a trip.
func (s *tripService) AssignDriver(
	ctx context.Context,
	id string,
	driverID string,
	vehicleID string,
) error {
	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	if driverID == "" {
		return fmt.Errorf("driver ID is required")
	}

	if vehicleID == "" {
		return fmt.Errorf("vehicle ID is required")
	}

	trip, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get trip for assignment: %w", err)
	}

	// A completed or cancelled trip cannot be assigned.
	switch trip.Status {
	case StatusCompleted:
		return fmt.Errorf("cannot assign driver to completed trip")

	case StatusCancelled:
		return fmt.Errorf("cannot assign driver to cancelled trip")
	}

	return s.repo.AssignDriver(
		ctx,
		id,
		driverID,
		vehicleID,
	)
}