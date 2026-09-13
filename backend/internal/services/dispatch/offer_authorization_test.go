package dispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type authorizationOfferRepository struct {
	repository.DispatchOfferRepository

	offer *models.DispatchOffer
	err   error

	requestedOfferID string
}

func (r *authorizationOfferRepository) GetByID(
	_ context.Context,
	offerID string,
) (*models.DispatchOffer, error) {
	r.requestedOfferID = offerID

	if r.err != nil {
		return nil, r.err
	}

	return r.offer, nil
}

type authorizationDriverRepository struct {
	repository.DriverRepository

	driver *models.Driver
	err    error

	requestedUserID string
}

func (r *authorizationDriverRepository) GetByUserID(
	_ context.Context,
	userID string,
) (*models.Driver, error) {
	r.requestedUserID = userID

	if r.err != nil {
		return nil, r.err
	}

	return r.driver, nil
}

func TestAcceptOfferAuthorizedDeniesUserWithoutDriverProfile(
	t *testing.T,
) {
	const (
		offerID       = "offer-123"
		ownerDriverID = "driver-owner"
		userID        = "non-driver-user"
	)

	offerRepo := &authorizationOfferRepository{
		offer: &models.DispatchOffer{
			ID:       offerID,
			DriverID: ownerDriverID,
		},
	}

	driverRepo := &authorizationDriverRepository{
		err: repository.ErrNotFound,
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
			Offers:  offerRepo,
		},
	)

	trip, err := service.AcceptOfferAuthorized(
		context.Background(),
		offerID,
		userID,
	)

	if !errors.Is(
		err,
		ErrDispatchOfferAccessDenied,
	) {
		t.Fatalf(
			"expected ErrDispatchOfferAccessDenied, got trip=%v err=%v",
			trip,
			err,
		)
	}

	if trip != nil {
		t.Fatalf(
			"expected nil trip, got %+v",
			trip,
		)
	}

	if offerRepo.requestedOfferID != offerID {
		t.Fatalf(
			"expected offer lookup for %q, got %q",
			offerID,
			offerRepo.requestedOfferID,
		)
	}

	if driverRepo.requestedUserID != userID {
		t.Fatalf(
			"expected driver lookup for user %q, got %q",
			userID,
			driverRepo.requestedUserID,
		)
	}
}

func TestAcceptOfferAuthorizedDeniesDriverWhoDoesNotOwnOffer(
	t *testing.T,
) {
	const (
		offerID       = "offer-123"
		ownerDriverID = "driver-owner"
		otherDriverID = "driver-other"
		userID        = "user-other"
	)

	offerRepo := &authorizationOfferRepository{
		offer: &models.DispatchOffer{
			ID:       offerID,
			DriverID: ownerDriverID,
		},
	}

	driverRepo := &authorizationDriverRepository{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: otherDriverID,
			},
		},
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
			Offers:  offerRepo,
		},
	)

	trip, err := service.AcceptOfferAuthorized(
		context.Background(),
		offerID,
		userID,
	)

	if !errors.Is(
		err,
		ErrDispatchOfferAccessDenied,
	) {
		t.Fatalf(
			"expected ErrDispatchOfferAccessDenied, got trip=%v err=%v",
			trip,
			err,
		)
	}

	if trip != nil {
		t.Fatalf(
			"expected nil trip, got %+v",
			trip,
		)
	}
}

func TestRejectOfferAuthorizedDeniesUserWithoutDriverProfile(
	t *testing.T,
) {
	const (
		offerID       = "offer-123"
		ownerDriverID = "driver-owner"
		userID        = "non-driver-user"
	)

	offerRepo := &authorizationOfferRepository{
		offer: &models.DispatchOffer{
			ID:       offerID,
			DriverID: ownerDriverID,
		},
	}

	driverRepo := &authorizationDriverRepository{
		err: repository.ErrNotFound,
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
			Offers:  offerRepo,
		},
	)

	offer, err := service.RejectOfferAuthorized(
		context.Background(),
		offerID,
		userID,
		"not available",
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

	if offerRepo.requestedOfferID != offerID {
		t.Fatalf(
			"expected offer lookup for %q, got %q",
			offerID,
			offerRepo.requestedOfferID,
		)
	}

	if driverRepo.requestedUserID != userID {
		t.Fatalf(
			"expected driver lookup for user %q, got %q",
			userID,
			driverRepo.requestedUserID,
		)
	}
}

func TestRejectOfferAuthorizedDeniesDriverWhoDoesNotOwnOffer(
	t *testing.T,
) {
	const (
		offerID       = "offer-123"
		ownerDriverID = "driver-owner"
		otherDriverID = "driver-other"
		userID        = "user-other"
	)

	offerRepo := &authorizationOfferRepository{
		offer: &models.DispatchOffer{
			ID:       offerID,
			DriverID: ownerDriverID,
		},
	}

	driverRepo := &authorizationDriverRepository{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: otherDriverID,
			},
		},
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
			Offers:  offerRepo,
		},
	)

	offer, err := service.RejectOfferAuthorized(
		context.Background(),
		offerID,
		userID,
		"not available",
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
}
