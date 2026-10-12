package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// FlexAuctionRepository defines persistence operations for negotiated-fare auctions.
//
// Lifecycle eligibility and ownership checks belong to the service layer.
// Operations requiring atomicity must use a transaction-bound repository.
type FlexAuctionRepository interface {
	Create(
		ctx context.Context,
		auction *models.FlexAuction,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.FlexAuction, error)

	GetByRideRequestID(
		ctx context.Context,
		rideRequestID string,
	) (*models.FlexAuction, error)

	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.FlexAuction, error)
}
