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

	// GetByIDForCompanyMember returns a vehicle only when the user has
	// explicit membership in the vehicle's company.
	GetByIDForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) (*models.Vehicle, error)

	// List returns all non-deleted vehicles across all companies.
	// This global surface is reserved for explicitly authorized callers.
	List(
		ctx context.Context,
	) ([]models.Vehicle, error)

	// ListForCompanyMember returns non-deleted vehicles belonging only to
	// companies in which the user has explicit membership.
	ListForCompanyMember(
		ctx context.Context,
		userID string,
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
