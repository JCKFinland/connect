package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *paymentService) CreateForCompletedTripAuthorized(
	ctx context.Context,
	tripID string,
	paymentMethod string,
	userID string,
) (*models.Payment, error) {
	if tripID == "" {
		return nil, fmt.Errorf(
			"trip ID is required",
		)
	}

	if userID == "" {
		return nil, fmt.Errorf(
			"user ID is required",
		)
	}

	if s.trips == nil {
		return nil, errors.New(
			"trip repository is not configured",
		)
	}

	trip, err :=
		s.trips.GetByID(
			ctx,
			tripID,
		)
	if err != nil {
		return nil, err
	}

	roles, err :=
		s.getUserRoles(
			ctx,
			userID,
		)
	if err != nil {
		return nil, err
	}

	if !canCreatePaymentForTrip(
		roles,
		userID,
		trip,
	) {
		return nil, ErrPaymentAccessDenied
	}

	return s.CreateForCompletedTrip(
		ctx,
		tripID,
		paymentMethod,
	)
}
