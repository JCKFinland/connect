package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// DriverVehicleAssignmentRepository defines persistence operations
// for operational driver-vehicle assignments.
type DriverVehicleAssignmentRepository interface {

	// Assign creates a new operational assignment.
	Assign(
		ctx context.Context,
		assignment *models.DriverVehicleAssignment,
	) error

	// GetByID retrieves a single assignment.
	GetByID(
		ctx context.Context,
		id string,
	) (*models.DriverVehicleAssignment, error)

	// List returns all non-deleted assignments.
	List(
		ctx context.Context,
	) ([]models.DriverVehicleAssignment, error)

	// Release ends an active assignment.
	Release(
		ctx context.Context,
		id string,
	) error

	// Delete performs a soft delete.
	Delete(
		ctx context.Context,
		id string,
	) error
}