package flex_offer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

var (
	ErrPassengerOfferAccessDenied       = errors.New("FLEX passenger offer access denied")
	ErrPassengerOfferAuctionNotEligible = errors.New("FLEX auction is not eligible for passenger offer discovery")
)

// ListPassengerOffers returns currently pending, unexpired driver offers
// for an auction owned by the authenticated passenger.
//
// The service is read-only and does not reserve drivers, select winners,
// create trips, or authorize payments.
func (s *Service) ListPassengerOffers(
	ctx context.Context,
	rideRequestID string,
	auctionID string,
	passengerUserID string,
) ([]*models.FlexOffer, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("FLEX database is not configured")
	}

	if rideRequestID == "" || auctionID == "" || passengerUserID == "" {
		return nil, ErrPassengerOfferAccessDenied
	}

	requests := postgresrepo.NewRideRequestRepository(s.db)

	request, err := requests.GetByID(ctx, rideRequestID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("load FLEX ride request: %w", err)
	}

	// Do not query the auction or driver offers until ownership is established.
	if request.CustomerID != passengerUserID {
		return nil, ErrPassengerOfferAccessDenied
	}

	if request.MatchingMode != models.RideMatchingModeFlex ||
		request.Status != "PENDING" {
		return nil, ErrPassengerOfferAuctionNotEligible
	}

	now := time.Now().UTC()

	if request.ExpiresAt != nil && !now.Before(request.ExpiresAt.UTC()) {
		return nil, ErrPassengerOfferAuctionNotEligible
	}

	auctions := postgresrepo.NewFlexAuctionRepository(s.db)

	auction, err := auctions.GetByID(ctx, auctionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("load FLEX auction: %w", err)
	}

	if auction.RideRequestID != rideRequestID {
		return nil, repository.ErrNotFound
	}

	if auction.Status != models.FlexAuctionStatusOpen ||
		auction.SelectedOfferID != nil ||
		!now.Before(auction.ExpiresAt.UTC()) {
		return nil, ErrPassengerOfferAuctionNotEligible
	}

	offers, err := postgresrepo.NewFlexOfferRepository(s.db).
		ListByAuctionID(ctx, auctionID)
	if err != nil {
		return nil, fmt.Errorf("list FLEX passenger offers: %w", err)
	}

	visible := make([]*models.FlexOffer, 0, len(offers))

	for _, offer := range offers {
		if offer == nil {
			continue
		}

		if offer.AuctionID != auctionID ||
			offer.Status != models.FlexOfferStatusPending ||
			!now.Before(offer.ExpiresAt.UTC()) {
			continue
		}

		visible = append(visible, offer)
	}

	return visible, nil
}
