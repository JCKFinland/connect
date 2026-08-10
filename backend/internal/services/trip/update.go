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