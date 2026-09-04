package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *paymentService) CreateForCompletedTrip(
	ctx context.Context,
	tripID string,
	paymentMethod string,
) (*models.Payment, error) {
	if tripID == "" {
		return nil, fmt.Errorf(
			"trip ID is required",
		)
	}

	if !isValidPaymentMethod(paymentMethod) {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrInvalidPaymentMethod,
			paymentMethod,
		)
	}

	if s.payments == nil {
		return nil, errors.New(
			"payment repository is not configured",
		)
	}

	payment, err :=
		s.payments.CreateFromCompletedTrip(
			ctx,
			tripID,
			paymentMethod,
		)
	if err == nil {
		return payment, nil
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" {

		existing, existingErr :=
			s.payments.GetByTripID(
				ctx,
				tripID,
			)

		if existingErr == nil &&
			existing != nil {

			return existing, nil
		}

		return nil, ErrPaymentAlreadyExists
	}

	if errors.Is(
		err,
		repository.ErrNotFound,
	) {
		return nil, repository.ErrNotFound
	}

	return nil, fmt.Errorf(
		"create payment: %w",
		err,
	)
}
