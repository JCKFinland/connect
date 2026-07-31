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
}