package repository

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// RideRequestRepository defines persistence operations for ride requests.
type RideRequestRepository interface {
	// Create persists a new ride request.
	Create(ctx context.Context, request *models.RideRequest) error

	// GetByID retrieves a ride request by ID.
	GetByID(ctx context.Context, id string) (*models.RideRequest, error)

	// List retrieves ride requests using optional filters and pagination.
	List(
		ctx context.Context,
		customerID string,
		status string,
		limit int,
		offset int,
	) ([]*models.RideRequest, error)

	// Update persists changes to a ride request.
	Update(ctx context.Context, request *models.RideRequest) error

	// Delete performs a soft delete where supported.
	Delete(ctx context.Context, id string) error

	// UpdateStatus changes the lifecycle status of a ride request.
	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
	) error

	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.RideRequest, error)

	ScheduleDispatchRetry(
		ctx context.Context,
		rideRequestID string,
		attemptedAt time.Time,
	) (int, time.Time, error)

	ResetDispatchRetry(
		ctx context.Context,
		rideRequestID string,
	) error
}
