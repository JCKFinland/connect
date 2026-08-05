package driver

import (
	"context"
)

// Delete performs a soft delete of a driver.
func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {

	// Ensure the driver exists before deleting.
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