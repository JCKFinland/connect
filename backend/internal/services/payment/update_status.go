package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

func (s *paymentService) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) (*models.Payment, error) {
	if id == "" {
		return nil, fmt.Errorf(
			"payment ID is required",
		)
	}

	if !isValidStatus(status) {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrInvalidPaymentStatus,
			status,
		)
	}

	if s.db == nil {
		return nil, errors.New(
			"payment database is not configured",
		)
	}

	var updatedPayment *models.Payment

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			payments :=
				postgresrepo.NewPaymentRepositoryWithDB(
					tx,
				)

			current, err :=
				payments.GetByIDForUpdate(
					ctx,
					id,
				)
			if err != nil {
				return fmt.Errorf(
					"get payment for status update: %w",
					err,
				)
			}

			if current.Status == status {
				updatedPayment = current
				return nil
			}

			if !canTransition(
				current.Status,
				status,
			) {
				return fmt.Errorf(
					"%w: %s -> %s",
					ErrInvalidPaymentTransition,
					current.Status,
					status,
				)
			}

			if err := payments.UpdateStatus(
				ctx,
				id,
				status,
			); err != nil {
				return fmt.Errorf(
					"persist payment status: %w",
					err,
				)
			}

			updatedPayment, err =
				payments.GetByID(
					ctx,
					id,
				)
			if err != nil {
				return fmt.Errorf(
					"reload updated payment: %w",
					err,
				)
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return updatedPayment, nil
}
