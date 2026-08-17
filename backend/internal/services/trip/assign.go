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

	currentTrip, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return fmt.Errorf(
			"get trip for assignment: %w",
			err,
		)
	}

	switch currentTrip.Status {

	case StatusCompleted:
		return fmt.Errorf(
			"cannot assign driver to completed trip",
		)

	case StatusCancelled:
		return fmt.Errorf(
			"cannot assign driver to cancelled trip",
		)
	}

	return s.repo.AssignDriver(
		ctx,
		id,
		driverID,
		vehicleID,
	)
}

// AssignDriverAuthorized assigns a driver and vehicle only when the
// authenticated user has operational trip-management privileges.
func (s *tripService) AssignDriverAuthorized(
	ctx context.Context,
	id string,
	driverID string,
	vehicleID string,
	userID string,
) error {

	if err := s.authorizeOperationalMutation(
		ctx,
		userID,
	); err != nil {
		return err
	}

	return s.AssignDriver(
		ctx,
		id,
		driverID,
		vehicleID,
	)
}
