package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type DriverPresenceRepository interface {
	Create(
		ctx context.Context,
		presence *models.DriverPresence,
	) error

	GetByDriverID(
		ctx context.Context,
		driverID string,
	) (*models.DriverPresence, error)

	// GetAvailableByDriverIDForUpdate locks and returns one currently
	// available driver for dispatch.
	//
	// Implementations should return repository.ErrNotFound when the driver
	// is unavailable or already locked by another dispatch transaction.
	GetAvailableByDriverIDForUpdate(
		ctx context.Context,
		driverID string,
	) (*models.DriverPresence, error)

	Update(
		ctx context.Context,
		presence *models.DriverPresence,
	) error

	UpdateHeartbeat(
		ctx context.Context,
		driverID string,
		latitude float64,
		longitude float64,
		heading float64,
		speed float64,
		accuracy float64,
	) error

	UpdateAvailability(
		ctx context.Context,
		driverID string,
		status string,
		isOnline bool,
	) error

	// UpdateAvailabilityIfIdle updates a driver's manually controlled
	// availability only when the driver is not committed to an active trip.
	//
	// It returns false without modifying presence when the driver is BUSY
	// or has an active non-terminal trip.
	UpdateAvailabilityIfIdle(
		ctx context.Context,
		driverID string,
		status string,
		isOnline bool,
	) (bool, error)

	SetOffline(
		ctx context.Context,
		driverID string,
	) error

	Delete(
		ctx context.Context,
		driverID string,
	) error

	ListAvailable(
		ctx context.Context,
		companyID string,
	) ([]*models.DriverPresence, error)

	ListAllAvailable(
		ctx context.Context,
	) ([]*models.DriverPresence, error)
}
