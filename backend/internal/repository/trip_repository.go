package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// TripRepository defines persistence operations for trips.
type TripRepository interface {
	// Create persists a new trip.
	Create(ctx context.Context, trip *models.Trip) error

	// GetByID retrieves a trip by its ID.
	GetByID(ctx context.Context, id string) (*models.Trip, error)

	// List retrieves trips using optional filtering and pagination.
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

	// Update persists changes to an existing trip.
	Update(ctx context.Context, trip *models.Trip) error

	// Delete performs a soft delete.
	Delete(ctx context.Context, id string) error

	// UpdateStatus changes the lifecycle state of a trip.
	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
	) error

	// UpdateActualMetrics persists the authoritative trip meter values.
	UpdateActualMetrics(
		ctx context.Context,
		id string,
		actualDistanceMeters int64,
		actualDurationSeconds int64,
	) error

	// AssignDriver assigns a driver and vehicle to a trip.
	AssignDriver(
		ctx context.Context,
		id string,
		driverID string,
		vehicleID string,
	) error
}
