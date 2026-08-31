package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type TripMeterMeasurementRepository interface {
	Create(
		ctx context.Context,
		measurement *models.TripMeterMeasurement,
	) error

	GetByTripID(
		ctx context.Context,
		tripID string,
	) (*models.TripMeterMeasurement, error)
}
