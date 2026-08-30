package trip

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service defines Trip business operations.
type Service interface {
	Create(
		ctx context.Context,
		trip *models.Trip,
	) error

	CreateAuthorized(
		ctx context.Context,
		trip *models.Trip,
		userID string,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Trip, error)

	GetByIDAuthorized(
		ctx context.Context,
		tripID string,
		userID string,
	) (*models.Trip, error)

	List(
		ctx context.Context,
		companyID string,
		branchID string,
		status string,
		driverID string,
		customerID string,
		limit int,
		offset int,
	) ([]*models.Trip, error)

	ListAuthorized(
		ctx context.Context,
		userID string,
		companyID string,
		branchID string,
		status string,
		driverID string,
		customerID string,
		limit int,
		offset int,
	) ([]*models.Trip, error)

	Update(
		ctx context.Context,
		trip *models.Trip,
	) error

	Delete(
		ctx context.Context,
		id string,
	) error

	DeleteAuthorized(
		ctx context.Context,
		id string,
		userID string,
	) error

	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
		performedByUserID string,
	) error

	AssignDriver(
		ctx context.Context,
		id string,
		driverID string,
		vehicleID string,
	) error

	AssignDriverAuthorized(
		ctx context.Context,
		id string,
		driverID string,
		vehicleID string,
		userID string,
	) error

	ListEvents(
		ctx context.Context,
		tripID string,
		userID string,
	) ([]*models.TripEvent, error)

	UpdateAuthorized(
		ctx context.Context,
		trip *models.Trip,
		userID string,
	) error

	CompleteTrip(
		ctx context.Context,
		tripID string,
		actorUserID string,
	) (*models.TripFare, error)
}

// Dependencies contains the resources required by the trip service.
type Dependencies struct {
	DB *pgxpool.Pool

	Trips        repository.TripRepository
	RideRequests repository.RideRequestRepository
	Presence     repository.DriverPresenceRepository
	TripEvents   repository.TripEventRepository
	UserRoles    repository.UserRoleRepository

	FareCalculator fare.Service
}

// tripService implements Service.
type tripService struct {
	db *pgxpool.Pool

	repo         repository.TripRepository
	rideRequests repository.RideRequestRepository
	presence     repository.DriverPresenceRepository
	tripEvents   repository.TripEventRepository
	userRoles    repository.UserRoleRepository

	fareCalculator fare.Service
}

// NewService creates a new Trip service.
func NewService(
	deps Dependencies,
) Service {
	return &tripService{
		db: deps.DB,

		repo:         deps.Trips,
		rideRequests: deps.RideRequests,
		presence:     deps.Presence,
		tripEvents:   deps.TripEvents,
		userRoles:    deps.UserRoles,

		fareCalculator: deps.FareCalculator,
	}
}

var _ Service = (*tripService)(nil)
