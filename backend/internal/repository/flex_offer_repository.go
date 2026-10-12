package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// FlexOfferRepository defines persistence operations for driver bids.
//
// Assignment eligibility and offer expiration must be validated by the
// transactional service before a bid is created or selected.
type FlexOfferRepository interface {
	Create(
		ctx context.Context,
		offer *models.FlexOffer,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.FlexOffer, error)

	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.FlexOffer, error)

	ListByAuctionID(
		ctx context.Context,
		auctionID string,
	) ([]*models.FlexOffer, error)
}
