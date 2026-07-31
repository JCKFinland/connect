package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type DriverAssignmentRepository interface {

	Create(
		ctx context.Context,
		assignment *models.DriverAssignment,
	) error

	Update(
		ctx context.Context,
		assignment *models.DriverAssignment,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.DriverAssignment, error)

	GetActiveByDriver(
		ctx context.Context,
		driverID string,
	) (*models.DriverAssignment, error)

	GetActiveByVehicle(
		ctx context.Context,
		vehicleID string,
	) (*models.DriverAssignment, error)

	ListByDriver(
		ctx context.Context,
		driverID string,
	) ([]*models.DriverAssignment, error)

	ListByVehicle(
		ctx context.Context,
		vehicleID string,
	) ([]*models.DriverAssignment, error)

	CloseAssignment(
		ctx context.Context,
		driverID string,
	) error

	Delete(
		ctx context.Context,
		id string,
	) error
}