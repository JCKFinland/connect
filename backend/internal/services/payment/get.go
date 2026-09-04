package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *paymentService) GetByID(
	ctx context.Context,
	id string,
) (*models.Payment, error) {
	if id == "" {
		return nil, fmt.Errorf(
			"payment ID is required",
		)
	}

	if s.payments == nil {
		return nil, errors.New(
			"payment repository is not configured",
		)
	}

	return s.payments.GetByID(
		ctx,
		id,
	)
}

func (s *paymentService) GetByTripID(
	ctx context.Context,
	tripID string,
) (*models.Payment, error) {
	if tripID == "" {
		return nil, fmt.Errorf(
			"trip ID is required",
		)
	}

	if s.payments == nil {
		return nil, errors.New(
			"payment repository is not configured",
		)
	}

	return s.payments.GetByTripID(
		ctx,
		tripID,
	)
}
