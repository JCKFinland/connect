package trip

import (
	"context"
	"fmt"
)

// Delete soft-deletes a trip.
func (s *tripService) Delete(
	ctx context.Context,
	id string,
) error {

	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	return s.repo.Delete(
		ctx,
		id,
	)
}

// DeleteAuthorized soft-deletes a trip only when the authenticated
// user has operational trip-management privileges.
func (s *tripService) DeleteAuthorized(
	ctx context.Context,
	id string,
	userID string,
) error {

	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	if err := s.authorizeOperationalMutation(
		ctx,
		userID,
	); err != nil {
		return err
	}

	// Confirm that the trip exists before attempting deletion.
	if _, err := s.repo.GetByID(
		ctx,
		id,
	); err != nil {
		return err
	}

	return s.repo.Delete(
		ctx,
		id,
	)
}
