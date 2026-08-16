package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// TripEventRepository defines persistence operations for immutable trip events.
type TripEventRepository interface {

	// Create stores a new trip event.
	Create(
		ctx context.Context,
		event *models.TripEvent,
	) error

	// ListByTripID returns all events for a trip in chronological order.
	ListByTripID(
		ctx context.Context,
		tripID string,
	) ([]*models.TripEvent, error)
}
