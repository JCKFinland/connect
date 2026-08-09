package ride_request

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID retrieves a ride request by ID.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*models.RideRequest, error) {
	return s.repo.GetByID(ctx, id)
}
