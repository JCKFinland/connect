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

	// UpdateHeartbeatIfOnline updates live driver location only when the
	// driver currently has an online operational presence.
	//
	// It returns false without modifying the row when the driver exists
	// but is offline or otherwise not in an online operational state.
	UpdateHeartbeatIfOnline(
		ctx context.Context,
		driverID string,
		latitude float64,
		longitude float64,
		heading float64,
		speed float64,
		accuracy float64,
	) (bool, error)

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

	// GetByDriverIDForUpdate retrieves and locks a driver's presence row
	// for the lifetime of the current PostgreSQL transaction.
	GetByDriverIDForUpdate(
		ctx context.Context,
		driverID string,
	) (*models.DriverPresence, error)

	// DetachAssignmentIfIdle removes the driver's vehicle/assignment presence
	// state only when there is no active trip and the driver is not BUSY.
	//
	// It returns false without modifying presence when detachment is unsafe.
	DetachAssignmentIfIdle(
		ctx context.Context,
		driverID string,
	) (bool, error)

	AttachAssignmentIfIdle(
		ctx context.Context,
		driverID string,
		companyID string,
		branchID string,
		vehicleID string,
		assignmentID string,
	) (bool, error)
}
