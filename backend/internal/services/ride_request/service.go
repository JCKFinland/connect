package ride_request

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Dependencies contains dependencies required by the ride-request service.
type Dependencies struct {
	Config       *config.Config
	RideRequests repository.RideRequestRepository
	UserRoles    repository.UserRoleRepository
}

// Service provides ride-request business operations.
type Service struct {
	cfg       *config.Config
	repo      repository.RideRequestRepository
	userRoles repository.UserRoleRepository
}

// NewService creates a new ride-request service.
func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		cfg:       deps.Config,
		repo:      deps.RideRequests,
		userRoles: deps.UserRoles,
	}
}
