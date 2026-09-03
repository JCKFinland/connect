package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// PaymentRepository defines persistence operations for payments.
type PaymentRepository interface {
	// CreateFromCompletedTrip creates the logical payment obligation
	// directly from the completed trip and its authoritative trip fare.
	//
	// Customer, fare, amount, and currency are derived by PostgreSQL
	// and are never supplied by the caller.
	CreateFromCompletedTrip(
		ctx context.Context,
		tripID string,
		paymentMethod string,
	) (*models.Payment, error)

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Payment, error)

	GetByTripID(
		ctx context.Context,
		tripID string,
	) (*models.Payment, error)
}
