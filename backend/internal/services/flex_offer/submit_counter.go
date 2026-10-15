package flex_offer

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// SubmitCounterOffer submits a driver's proposed fare to an
// existing FLEX auction.
//
// The shared transactional workflow enforces driver eligibility,
// auction lifecycle, immutable pricing boundaries, and uniqueness.
//
// Submission does not select a winning offer, create a trip,
// authorize payment, or change driver availability.
func (s *Service) SubmitCounterOffer(
	ctx context.Context,
	auctionID string,
	driverUserID string,
	proposedAmountCents int64,
) (*models.FlexOffer, error) {
	if proposedAmountCents <= 0 {
		return nil, ErrInvalidCounterOfferAmount
	}

	return s.submitOffer(
		ctx,
		auctionID,
		driverUserID,
		models.FlexOfferTypeCounterOffer,
		proposedAmountCents,
	)
}
