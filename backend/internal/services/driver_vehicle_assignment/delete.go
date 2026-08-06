package driver_vehicle_assignment

import (
	"context"
)

// Delete performs a soft delete of an assignment.
func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {

	_, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return err
	}

	return s.repo.Delete(
		ctx,
		id,
	)
}