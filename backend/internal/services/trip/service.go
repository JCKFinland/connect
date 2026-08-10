package trip

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Service defines Trip business operations.
type Service interface {
	Create(ctx context.Context, trip *models.Trip) error

	GetByID(ctx context.Context, id string) (*models.Trip, error)

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

	Update(ctx context.Context, trip *models.Trip) error

	Delete(ctx context.Context, id string) error

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

// tripService implements Service.
type tripService struct {
	repo repository.TripRepository
}

// NewService creates a new Trip service.
func NewService(repo repository.TripRepository) Service {
	return &tripService{
		repo: repo,
	}
}

var _ Service = (*tripService)(nil)
