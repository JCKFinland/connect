package ride_request

import "github.com/JCKFinland/connect/backend/internal/repository"

// Service provides ride request business operations.
type Service struct {
	repo repository.RideRequestRepository
}

// NewService creates a new ride request service.
func NewService(
	repo repository.RideRequestRepository,
) *Service {
	return &Service{
		repo: repo,
	}
}
