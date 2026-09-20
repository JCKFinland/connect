package dispatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrPendingDispatchOfferNotFound = errors.New(
	"no pending dispatch offer found",
)

// GetPendingOfferAuthorized returns the authenticated driver's
// currently active PENDING dispatch offer together with the ride
// information required to make an accept/reject decision.
//
// The authenticated user is first resolved to a driver profile.
// This prevents one driver from retrieving another driver's offer.
func (s *Service) GetPendingOfferAuthorized(
	ctx context.Context,
	userID string,
) (*PendingOfferResponse, error) {
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

	ride, err := s.rideRequests.GetByID(
		ctx,
		offer.RideRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get dispatch offer ride request: %w",
			err,
		)
	}

	if ride == nil {
		return nil, errors.New(
			"dispatch offer ride request not found",
		)
	}

	return &PendingOfferResponse{
		ID:            offer.ID,
		RideRequestID: offer.RideRequestID,
		VehicleID:     offer.VehicleID,
		Status:        offer.Status,
		OfferedAt:     offer.OfferedAt,
		ExpiresAt:     offer.ExpiresAt,
		Ride: PendingOfferRide{
			PickupAddress:        ride.PickupAddress,
			PickupLatitude:       ride.PickupLatitude,
			PickupLongitude:      ride.PickupLongitude,
			DestinationAddress:   ride.DestinationAddress,
			DestinationLatitude:  ride.DestinationLatitude,
			DestinationLongitude: ride.DestinationLongitude,
			RequestedVehicleType: ride.RequestedVehicleType,
			ServiceCategoryID:    ride.ServiceCategoryID,
			PassengerCount:       ride.PassengerCount,
			Notes:                ride.Notes,
			RequestedAt:          ride.RequestedAt,
		},
	}, nil
}
