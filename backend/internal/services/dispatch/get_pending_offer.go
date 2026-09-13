package dispatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrPendingDispatchOfferNotFound = errors.New(
	"no pending dispatch offer found",
)

// GetPendingOfferAuthorized returns the authenticated driver's
// currently active PENDING dispatch offer.
//
// The authenticated user is first resolved to a driver profile.
// This prevents one driver from retrieving another driver's offer.
func (s *Service) GetPendingOfferAuthorized(
	ctx context.Context,
	userID string,
) (*models.DispatchOffer, error) {
	if userID == "" {
		return nil, errors.New(
			"authenticated user ID is required",
		)
	}

	driver, err := s.drivers.GetByUserID(
		ctx,
		userID,
	)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDispatchOfferAccessDenied
		}

		return nil, fmt.Errorf(
			"resolve authenticated driver: %w",
			err,
		)
	}

	if driver == nil || driver.ID == "" {
		return nil, ErrDispatchOfferAccessDenied
	}

	offer, err := s.offers.GetPendingByDriver(
		ctx,
		driver.ID,
	)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPendingDispatchOfferNotFound
		}

		return nil, fmt.Errorf(
			"get pending dispatch offer: %w",
			err,
		)
	}

	if offer == nil {
		return nil, ErrPendingDispatchOfferNotFound
	}

	return offer, nil
}
