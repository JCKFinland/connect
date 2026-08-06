package driver_vehicle_assignment

import (
	"context"
)

// Release ends an active driver-vehicle assignment.
func (s *Service) Release(
	ctx context.Context,
	id string,
) error {

	assignment, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return err
	}

	if assignment.Status == "RELEASED" {
		return ErrAssignmentAlreadyReleased
	}

	return s.repo.Release(
		ctx,
		id,
	)
}