package ride_request

import "github.com/JCKFinland/connect/backend/internal/repository"

// Dependencies contains repositories required by the ride-request service.
type Dependencies struct {
	RideRequests repository.RideRequestRepository
	UserRoles    repository.UserRoleRepository
}

// Service provides ride-request business operations.
type Service struct {
	repo      repository.RideRequestRepository
	userRoles repository.UserRoleRepository
}

// NewService creates a new ride-request service.
func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		repo:      deps.RideRequests,
		userRoles: deps.UserRoles,
	}
}
