package repository

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// TripLocationRepository defines persistence operations for immutable
// trip GPS/location samples.
type TripLocationRepository interface {

	// Create stores a new location sample.
	Create(
		ctx context.Context,
		location *models.TripLocation,
	) error

	// GetByObservationIdentity returns the persisted sample identified by
	// trip, driver, and the device-recorded timestamp.
	GetByObservationIdentity(
		ctx context.Context,
		tripID string,
		driverID string,
		recordedAt time.Time,
	) (*models.TripLocation, error)

	// ListByTripID returns all location samples for a trip
	// in chronological order.
	ListByTripID(
		ctx context.Context,
		tripID string,
	) ([]*models.TripLocation, error)
}
