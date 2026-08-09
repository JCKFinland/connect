package ride_request

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Update modifies an existing ride request.
func (s *Service) Update(
	ctx context.Context,
	request *models.RideRequest,
) error {
	return s.repo.Update(
		ctx,
		request,
	)
}
