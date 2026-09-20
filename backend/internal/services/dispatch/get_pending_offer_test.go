package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// pendingOfferDriverRepository is a focused test double.
//
// Embedding DriverRepository supplies the rest of the interface while this
// test overrides only GetByUserID, which is the sole driver operation used by
// GetPendingOfferAuthorized.
type pendingOfferDriverRepository struct {
	repository.DriverRepository

	driver *models.Driver
	err    error

	requestedUserID string
}

func (r *pendingOfferDriverRepository) GetByUserID(
	_ context.Context,
	userID string,
) (*models.Driver, error) {
	r.requestedUserID = userID

	if r.err != nil {
		return nil, r.err
	}

	return r.driver, nil
}

// pendingOfferRepository is a focused test double.
//
// GetPendingByDriver is the only dispatch-offer repository operation used by
// GetPendingOfferAuthorized.
type pendingOfferRepository struct {
	repository.DispatchOfferRepository

	offer *models.DispatchOffer
	err   error

	requestedDriverID string
	called            bool
}

type pendingOfferRideRequestRepository struct {
	repository.RideRequestRepository

	request *models.RideRequest
	err     error

	requestedRideRequestID string
	called                 bool
}

func (r *pendingOfferRideRequestRepository) GetByID(
	_ context.Context,
	id string,
) (*models.RideRequest, error) {
	r.called = true
	r.requestedRideRequestID = id

	if r.err != nil {
		return nil, r.err
	}

	return r.request, nil
}

func (r *pendingOfferRepository) GetPendingByDriver(
	_ context.Context,
	driverID string,
) (*models.DispatchOffer, error) {
	r.called = true
	r.requestedDriverID = driverID

	if r.err != nil {
		return nil, r.err
	}

	return r.offer, nil
}

func TestGetPendingOfferAuthorizedReturnsAuthenticatedDriversOffer(
	t *testing.T,
) {
	const (
		userID        = "user-123"
		driverID      = "driver-123"
		offerID       = "offer-123"
		rideRequestID = "ride-request-123"
	)

	now := time.Now().UTC()

	driverRepo := &pendingOfferDriverRepository{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: driverID,
			},
		},
	}

	offerRepo := &pendingOfferRepository{
		offer: &models.DispatchOffer{
			ID:            offerID,
			RideRequestID: rideRequestID,
			DriverID:      driverID,
			Status:        "PENDING",
			OfferedAt:     now,
			ExpiresAt:     now.Add(30 * time.Second),
		},
	}

	rideRepo := &pendingOfferRideRequestRepository{
		request: &models.RideRequest{
			PickupAddress:        "Pickup Street 1",
			PickupLatitude:       60.2055,
			PickupLongitude:      24.6559,
			DestinationAddress:   "Destination Street 2",
			DestinationLatitude:  60.1699,
			DestinationLongitude: 24.9384,
			RequestedVehicleType: "VAN",
			PassengerCount:       2,
			Notes:                "Two bags",
			RequestedAt:          now.Add(-time.Minute),
		},
	}

	service := NewService(
		Dependencies{
			Drivers:      driverRepo,
			Offers:       offerRepo,
			RideRequests: rideRepo,
		},
	)

	offer, err := service.GetPendingOfferAuthorized(
		context.Background(),
		userID,
	)
	if err != nil {
		t.Fatalf(
			"GetPendingOfferAuthorized returned unexpected error: %v",
			err,
		)
	}

	if offer == nil {
		t.Fatal(
			"expected pending dispatch offer, got nil",
		)
	}

	if offer.ID != offerID {
		t.Fatalf(
			"expected offer ID %q, got %q",
			offerID,
			offer.ID,
		)
	}

	if offer.RideRequestID != rideRequestID {
		t.Fatalf(
			"expected ride request ID %q, got %q",
			rideRequestID,
			offer.RideRequestID,
		)
	}

	if offer.Ride.PickupAddress != "Pickup Street 1" {
		t.Fatalf(
			"expected pickup address %q, got %q",
			"Pickup Street 1",
			offer.Ride.PickupAddress,
		)
	}

	if offer.Ride.DestinationAddress != "Destination Street 2" {
		t.Fatalf(
			"expected destination address %q, got %q",
			"Destination Street 2",
			offer.Ride.DestinationAddress,
		)
	}

	if offer.Ride.PassengerCount != 2 {
		t.Fatalf(
			"expected passenger count 2, got %d",
			offer.Ride.PassengerCount,
		)
	}

	if !rideRepo.called {
		t.Fatal("expected ride-request repository lookup")
	}

	if rideRepo.requestedRideRequestID != rideRequestID {
		t.Fatalf(
			"expected ride-request lookup for %q, got %q",
			rideRequestID,
			rideRepo.requestedRideRequestID,
		)
	}

	if driverRepo.requestedUserID != userID {
		t.Fatalf(
			"expected driver lookup for user %q, got %q",
			userID,
			driverRepo.requestedUserID,
		)
	}

	if !offerRepo.called {
		t.Fatal(
			"expected pending-offer repository lookup",
		)
	}

	if offerRepo.requestedDriverID != driverID {
		t.Fatalf(
			"expected pending offer lookup for driver %q, got %q",
			driverID,
			offerRepo.requestedDriverID,
		)
	}
}

func TestGetPendingOfferAuthorizedReturnsNotFoundWhenDriverHasNoPendingOffer(
	t *testing.T,
) {
	const (
		userID   = "user-123"
		driverID = "driver-123"
	)

	driverRepo := &pendingOfferDriverRepository{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: driverID,
			},
		},
	}

	offerRepo := &pendingOfferRepository{
		err: repository.ErrNotFound,
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
			Offers:  offerRepo,
		},
	)

	offer, err := service.GetPendingOfferAuthorized(
		context.Background(),
		userID,
	)

	if !errors.Is(
		err,
		ErrPendingDispatchOfferNotFound,
	) {
		t.Fatalf(
			"expected ErrPendingDispatchOfferNotFound, got offer=%v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf(
			"expected nil offer, got %+v",
			offer,
		)
	}

	if offerRepo.requestedDriverID != driverID {
		t.Fatalf(
			"expected pending offer lookup for driver %q, got %q",
			driverID,
			offerRepo.requestedDriverID,
		)
	}
}

func TestGetPendingOfferAuthorizedDeniesUserWithoutDriverProfile(
	t *testing.T,
) {
	const userID = "non-driver-user"

	driverRepo := &pendingOfferDriverRepository{
		err: repository.ErrNotFound,
	}

	offerRepo := &pendingOfferRepository{}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
			Offers:  offerRepo,
		},
	)

	offer, err := service.GetPendingOfferAuthorized(
		context.Background(),
		userID,
	)

	if !errors.Is(
		err,
		ErrDispatchOfferAccessDenied,
	) {
		t.Fatalf(
			"expected ErrDispatchOfferAccessDenied, got offer=%v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf(
			"expected nil offer, got %+v",
			offer,
		)
	}

	if offerRepo.called {
		t.Fatal(
			"pending-offer repository must not be queried when authenticated user has no driver profile",
		)
	}
}
