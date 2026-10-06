package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// DriverRepository defines all persistence operations for driver records.
type DriverRepository interface {

	// Create stores a new driver.
	Create(
		ctx context.Context,
		driver *models.Driver,
	) error

	// GetByID returns a single driver by its unique identifier.
	GetByID(
		ctx context.Context,
		id string,
	) (*models.Driver, error)

	// GetByIDForUpdate returns a driver and locks the row for the
	// duration of the current database transaction.
	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.Driver, error)

	// GetByUserID returns the driver associated with an authenticated user.
	GetByUserID(
		ctx context.Context,
		userID string,
	) (*models.Driver, error)

	// List returns all non-deleted drivers.
	List(
		ctx context.Context,
	) ([]models.Driver, error)

	// UpdateDetails persists only mutable profile and employment details.
	// It must not modify account identity, tenant/branch authority, regulatory
	// credentials, verification metadata, or lifecycle state.
	UpdateDetails(
		ctx context.Context,
		driver *models.Driver,
	) error

	// Update persists the complete driver record for dedicated lifecycle
	// workflows such as atomic verification. Generic management updates must
	// use UpdateDetails instead.
	Update(
		ctx context.Context,
		driver *models.Driver,
	) error

	// Delete performs a soft delete on a driver.
	Delete(
		ctx context.Context,
		id string,
	) error
}
