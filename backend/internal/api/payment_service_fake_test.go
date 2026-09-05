package api

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type fakePaymentService struct {
	createAuthorized func(
		ctx context.Context,
		tripID string,
		paymentMethod string,
		userID string,
	) (*models.Payment, error)

	getByIDAuthorized func(
		ctx context.Context,
		id string,
		userID string,
	) (*models.Payment, error)

	getByTripIDAuthorized func(
		ctx context.Context,
		tripID string,
		userID string,
	) (*models.Payment, error)
}

func (f *fakePaymentService) CreateForCompletedTrip(
	context.Context,
	string,
	string,
) (*models.Payment, error) {
	panic("unexpected CreateForCompletedTrip call")
}

func (f *fakePaymentService) GetByID(
	context.Context,
	string,
) (*models.Payment, error) {
	panic("unexpected GetByID call")
}

func (f *fakePaymentService) GetByTripID(
	context.Context,
	string,
) (*models.Payment, error) {
	panic("unexpected GetByTripID call")
}

func (f *fakePaymentService) UpdateStatus(
	context.Context,
	string,
	string,
) (*models.Payment, error) {
	panic("unexpected UpdateStatus call")
}

func (f *fakePaymentService) CreateForCompletedTripAuthorized(
	ctx context.Context,
	tripID string,
	paymentMethod string,
	userID string,
) (*models.Payment, error) {
	if f.createAuthorized == nil {
		panic("unexpected CreateForCompletedTripAuthorized call")
	}

	return f.createAuthorized(
		ctx,
		tripID,
		paymentMethod,
		userID,
	)
}

func (f *fakePaymentService) GetByIDAuthorized(
	ctx context.Context,
	id string,
	userID string,
) (*models.Payment, error) {
	if f.getByIDAuthorized == nil {
		panic("unexpected GetByIDAuthorized call")
	}

	return f.getByIDAuthorized(
		ctx,
		id,
		userID,
	)
}

func (f *fakePaymentService) GetByTripIDAuthorized(
	ctx context.Context,
	tripID string,
	userID string,
) (*models.Payment, error) {
	if f.getByTripIDAuthorized == nil {
		panic("unexpected GetByTripIDAuthorized call")
	}

	return f.getByTripIDAuthorized(
		ctx,
		tripID,
		userID,
	)
}
