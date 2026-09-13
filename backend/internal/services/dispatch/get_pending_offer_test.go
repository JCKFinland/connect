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
		userID   = "user-123"
		driverID = "driver-123"
		offerID  = "offer-123"
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
			ID:        offerID,
			DriverID:  driverID,
			Status:    "PENDING",
			OfferedAt: now,
			ExpiresAt: now.Add(30 * time.Second),
		},
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
