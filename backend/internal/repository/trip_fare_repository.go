package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// TripFareRepository defines persistence operations for trip fares.
type TripFareRepository interface {
	// Create persists the authoritative fare for a trip.
	Create(
		ctx context.Context,
		fare *models.TripFare,
	) error

	// GetByID retrieves a fare by its ID.
	GetByID(
		ctx context.Context,
		id string,
	) (*models.TripFare, error)

	// GetByTripID retrieves the authoritative fare for a trip.
	GetByTripID(
		ctx context.Context,
		tripID string,
	) (*models.TripFare, error)
}
