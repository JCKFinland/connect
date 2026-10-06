package driver

import (
	"context"
)

// Delete performs a soft delete of a driver.
func (s *Service) Delete(
	ctx context.Context,
	userID string,
	id string,
) error {

	// Resolve the target through the caller's tenant authority boundary.
	_, err := s.resolveManagementTarget(
		ctx,
		userID,
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
