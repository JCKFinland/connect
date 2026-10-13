package ride_request

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	fareestimate "github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies contains dependencies required by the ride-request service.
type Dependencies struct {
	DB                *pgxpool.Pool
	Config            *config.Config
	RideRequests      repository.RideRequestRepository
	UserRoles         repository.UserRoleRepository
	ServiceCategories repository.ServiceCategoryRepository
	FareEstimator     fareestimate.Service
}

// Service provides ride-request business operations.
type Service struct {
	db                *pgxpool.Pool
	cfg               *config.Config
	repo              repository.RideRequestRepository
	userRoles         repository.UserRoleRepository
	serviceCategories repository.ServiceCategoryRepository
	fareEstimator     fareestimate.Service
}

// NewService creates a new ride-request service.
func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		db:                deps.DB,
		cfg:               deps.Config,
		repo:              deps.RideRequests,
		userRoles:         deps.UserRoles,
		serviceCategories: deps.ServiceCategories,
		fareEstimator:     deps.FareEstimator,
	}
}
