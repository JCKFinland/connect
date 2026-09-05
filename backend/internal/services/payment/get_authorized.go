package payment

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *paymentService) GetByIDAuthorized(
	ctx context.Context,
	id string,
	userID string,
) (*models.Payment, error) {
	if id == "" {
		return nil, fmt.Errorf(
			"payment ID is required",
		)
	}

	if userID == "" {
		return nil, fmt.Errorf(
			"user ID is required",
		)
	}

	payment, err :=
		s.GetByID(
			ctx,
			id,
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

	if !canReadPayment(
		roles,
		userID,
		payment,
	) {
		return nil, ErrPaymentAccessDenied
	}

	return payment, nil
}

func (s *paymentService) GetByTripIDAuthorized(
	ctx context.Context,
	tripID string,
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

	payment, err :=
		s.GetByTripID(
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

	if !canReadPayment(
		roles,
		userID,
		payment,
	) {
		return nil, ErrPaymentAccessDenied
	}

	return payment, nil
}
