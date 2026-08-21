package repository

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// DispatchOfferRepository defines persistence operations for ride dispatch offers.
type DispatchOfferRepository interface {
	Create(
		ctx context.Context,
		offer *models.DispatchOffer,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.DispatchOffer, error)

	GetPendingByDriver(
		ctx context.Context,
		driverID string,
	) (*models.DispatchOffer, error)

	GetPendingByRideRequest(
		ctx context.Context,
		rideRequestID string,
	) (*models.DispatchOffer, error)

	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.DispatchOffer, error)

	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
		respondedAt *time.Time,
		rejectionReason *string,
	) error

	ExpireStalePending(
		ctx context.Context,
		now time.Time,
	) ([]string, error)

	ListDriverIDsByRideRequest(
		ctx context.Context,
		rideRequestID string,
	) ([]string, error)

	ListRedispatchableRideRequestIDs(
		ctx context.Context,
		limit int,
	) ([]string, error)
}
