package trip

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service defines Trip business operations.
type Service interface {
	Create(
		ctx context.Context,
		trip *models.Trip,
	) error

	GetByID(
		ctx context.Context,
		id string,
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

	Update(
		ctx context.Context,
		trip *models.Trip,
	) error

	Delete(
		ctx context.Context,
		id string,
	) error

	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
	) error

	AssignDriver(
		ctx context.Context,
		id string,
		driverID string,
		vehicleID string,
	) error
}

// Dependencies contains the resources required by the trip service.
type Dependencies struct {
	DB *pgxpool.Pool

	Trips        repository.TripRepository
	RideRequests repository.RideRequestRepository
	Presence     repository.DriverPresenceRepository
}

// tripService implements Service.
type tripService struct {
	db *pgxpool.Pool

	repo         repository.TripRepository
	rideRequests repository.RideRequestRepository
	presence     repository.DriverPresenceRepository
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
	}
}

var _ Service = (*tripService)(nil)
