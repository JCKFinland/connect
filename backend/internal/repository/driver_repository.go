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
	// GetByID returns a single driver by its unique identifier.
	GetByID(
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

	// Update persists changes to an existing driver.
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
