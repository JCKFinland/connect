package ride_request

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List retrieves ride requests.
func (s *Service) List(
	ctx context.Context,
	customerID string,
	status string,
	limit int,
	offset int,
) ([]*models.RideRequest, error) {
	return s.repo.List(
		ctx,
		customerID,
		status,
		limit,
		offset,
	)
}
