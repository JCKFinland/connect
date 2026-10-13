package ride_request

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	"github.com/JCKFinland/connect/backend/internal/services/flex_pricing"
	"github.com/jackc/pgx/v5"
)

var (
	ErrFlexAuctionAccessDenied = errors.New("flex auction access denied")
	ErrFlexAuctionNotEligible  = errors.New("ride request is not eligible for flex auction")
	ErrFlexAuctionExists       = errors.New("flex auction already exists")
)

type InitiateFlexAuctionResult struct {
	Auction        *models.FlexAuction
	PricingBounds  flex_pricing.BidBounds
	PricingProfile string
	PricingVersion string
}

func (s *Service) InitiateFlexAuctionAuthorized(
	ctx context.Context,
	rideRequestID string,
	userID string,
	offerCents int64,
) (*InitiateFlexAuctionResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("ride-request database is not configured")
	}
	if s.fareEstimator == nil {
		return nil, errors.New("FLEX fare estimator is not configured")
	}
	if s.userRoles == nil {
		return nil, errors.New("ride-request role repository is not configured")
	}
	if rideRequestID == "" || userID == "" {
		return nil, ErrFlexAuctionAccessDenied
	}
	if offerCents <= 0 {
		return nil, flex_pricing.ErrInvalidOffer
	}

	// Read only the route and category needed for the external estimate.
	// Authoritative eligibility is checked again under locks below.
	request, err := postgresrepo.NewRideRequestRepository(s.db).
		GetByID(ctx, rideRequestID)
	if err != nil {
		return nil, fmt.Errorf("load FLEX auction request: %w", err)
	}
	if request.CustomerID != userID {
		return nil, ErrFlexAuctionAccessDenied
	}
	if request.Status != "PENDING" ||
		request.MatchingMode != models.RideMatchingModeFlex {
		return nil, ErrFlexAuctionNotEligible
	}
	if request.ServiceCategoryID == nil ||
		*request.ServiceCategoryID == "" {
		return nil, ErrFlexAuctionNotEligible
	}
	if request.ExpiresAt != nil &&
		!time.Now().UTC().Before(request.ExpiresAt.UTC()) {
		return nil, ErrFlexAuctionNotEligible
	}

	roles, err := s.getUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("authorize FLEX auction: %w", err)
	}
	isCustomer := false
	for _, role := range roles {
		if role == "CUSTOMER" {
			isCustomer = true
			break
		}
	}
	if !isCustomer {
		return nil, ErrFlexAuctionAccessDenied
	}

	estimate, err := s.fareEstimator.Estimate(
		ctx,
		fare_estimate.EstimateRequest{
			PickupLatitude:       request.PickupLatitude,
			PickupLongitude:      request.PickupLongitude,
			DestinationLatitude:  request.DestinationLatitude,
			DestinationLongitude: request.DestinationLongitude,
			ServiceCategoryID:    *request.ServiceCategoryID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("estimate FLEX auction fare: %w", err)
	}
	if estimate == nil {
		return nil, flex_pricing.ErrInvalidBaseline
	}

	bounds, err := flex_pricing.BoundsFromEstimate(
		estimate.TotalAmount,
		estimate.Currency,
	)
	if err != nil {
		return nil, err
	}
	if err := flex_pricing.ValidateOffer(offerCents, bounds); err != nil {
		return nil, err
	}

	var created *models.FlexAuction

	err = postgresrepo.RunInTransaction(ctx, s.db, func(tx pgx.Tx) error {
		if err := postgresrepo.AcquireTransactionAdvisoryLock(
			ctx,
			tx,
			"ride-dispatch:"+rideRequestID,
		); err != nil {
			return fmt.Errorf("lock FLEX auction initiation: %w", err)
		}

		requests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
		locked, err := requests.GetByIDForUpdate(ctx, rideRequestID)
		if err != nil {
			return fmt.Errorf("lock FLEX auction request: %w", err)
		}

		if locked.CustomerID != userID {
			return ErrFlexAuctionAccessDenied
		}
		if locked.Status != "PENDING" ||
			locked.MatchingMode != models.RideMatchingModeFlex ||
			locked.ServiceCategoryID == nil ||
			*locked.ServiceCategoryID == "" {
			return ErrFlexAuctionNotEligible
		}

		// Prevent persisting an estimate calculated for changed ride inputs.
		if locked.PickupLatitude != request.PickupLatitude ||
			locked.PickupLongitude != request.PickupLongitude ||
			locked.DestinationLatitude != request.DestinationLatitude ||
			locked.DestinationLongitude != request.DestinationLongitude ||
			*locked.ServiceCategoryID != *request.ServiceCategoryID {
			return ErrFlexAuctionNotEligible
		}

		now := time.Now().UTC()
		if locked.ExpiresAt != nil && !now.Before(locked.ExpiresAt.UTC()) {
			return ErrFlexAuctionNotEligible
		}

		auctions := postgresrepo.NewFlexAuctionRepositoryWithDB(tx)
		existing, err := auctions.GetByRideRequestID(ctx, rideRequestID)
		switch {
		case err == nil && existing != nil:
			return ErrFlexAuctionExists
		case err == nil:
			return errors.New("FLEX auction lookup returned nil auction")
		case !errors.Is(err, repository.ErrNotFound):
			return fmt.Errorf("check existing FLEX auction: %w", err)
		}

		expiresAt := now.Add(60 * time.Second)
		if locked.ExpiresAt != nil &&
			locked.ExpiresAt.Before(expiresAt) {
			expiresAt = locked.ExpiresAt.UTC()
		}
		if !expiresAt.After(now) {
			return ErrFlexAuctionNotEligible
		}

		auction := &models.FlexAuction{
			RideRequestID:       rideRequestID,
			PassengerOfferCents: offerCents,
			Currency:            "EUR",
			BaselineCents:       bounds.BaselineCents,
			MinimumOfferCents:   bounds.MinimumCents,
			MaximumOfferCents:   bounds.MaximumCents,
			PricingProfileID:    estimate.PricingProfileID,
			PricingVersion:      estimate.PricingVersion,
			OpenedAt:            now,
			ExpiresAt:           expiresAt,
		}

		if err := auctions.Create(ctx, auction); err != nil {
			return fmt.Errorf("persist FLEX auction: %w", err)
		}
		created = auction
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &InitiateFlexAuctionResult{
		Auction:        created,
		PricingBounds:  bounds,
		PricingProfile: estimate.PricingProfileID,
		PricingVersion: estimate.PricingVersion,
	}, nil
}
