package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Update updates editable trip information.
func (s *tripService) Update(
	ctx context.Context,
	trip *models.Trip,
) error {
	if trip == nil {
		return fmt.Errorf("trip is required")
	}

	if trip.ID == "" {
		return fmt.Errorf("trip ID is required")
	}

	// Protected fields such as driver, vehicle, company,
	// branch, fleet, customer, and status are intentionally
	// not changed by the repository Update operation.
	return s.repo.Update(ctx, trip)
}

// UpdateAuthorized updates editable trip information only when the
// authenticated user has operational trip-management privileges.
func (s *tripService) UpdateAuthorized(
	ctx context.Context,
	trip *models.Trip,
	userID string,
) error {

	if trip == nil {
		return fmt.Errorf("trip is required")
	}

	if trip.ID == "" {
		return fmt.Errorf("trip ID is required")
	}

	if err := s.authorizeOperationalMutation(
		ctx,
		userID,
	); err != nil {
		return err
	}

	// Confirm the trip exists before attempting the update.
	if _, err := s.repo.GetByID(
		ctx,
		trip.ID,
	); err != nil {
		return err
	}

	return s.repo.Update(
		ctx,
		trip,
	)
}
