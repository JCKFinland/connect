package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// VehicleRepository defines persistence operations for vehicles.
type VehicleRepository interface {

	// Create stores a new vehicle.
	Create(
		ctx context.Context,
		vehicle *models.Vehicle,
	) error

	// GetByID returns a vehicle by its ID.
	GetByID(
		ctx context.Context,
		id string,
	) (*models.Vehicle, error)

	// List returns all active vehicles.
	List(
		ctx context.Context,
	) ([]models.Vehicle, error)

	// Update modifies an existing vehicle.
	Update(
		ctx context.Context,
		vehicle *models.Vehicle,
	) error

	// Delete performs a soft delete.
	Delete(
		ctx context.Context,
		id string,
	) error
}
