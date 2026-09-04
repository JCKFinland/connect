package paymenttransaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

func (s *paymentTransactionService) ApplyResult(
	ctx context.Context,
	transactionID string,
	req ApplyResultRequest,
) (*models.PaymentTransaction, error) {
	if transactionID == "" {
		return nil, errors.New(
			"payment transaction ID is required",
		)
	}

	if !isValidStatus(req.Status) {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrInvalidTransactionStatus,
			req.Status,
		)
	}

	if s.db == nil {
		return nil, errors.New(
			"payment transaction database is not configured",
		)
	}

	var updated *models.PaymentTransaction

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			transactions :=
				postgresrepo.NewPaymentTransactionRepositoryWithDB(
					tx,
				)

			current, err :=
				transactions.GetByIDForUpdate(
					ctx,
					transactionID,
				)
			if err != nil {
				return fmt.Errorf(
					"get payment transaction for result application: %w",
					err,
				)
			}

			if current.ProviderTransactionID != nil &&
				req.ProviderTransactionID != nil &&
				*current.ProviderTransactionID !=
					*req.ProviderTransactionID {

				return ErrProviderIdentityConflict
			}

			// Exact duplicate delivery is idempotent.
			// Same-status provider delivery is idempotent.
			if current.Status == req.Status {
				// If CONNECT already has the provider identity, there is
				// nothing lifecycle-sensitive left to change.
				if current.ProviderTransactionID != nil ||
					req.ProviderTransactionID == nil {

					updated = current
					return nil
				}

				// A later duplicate delivery may legitimately supply the
				// provider transaction identifier that was unavailable on
				// the earlier delivery. Fill it exactly once without changing
				// the transaction lifecycle state.
				if err := transactions.UpdateResult(
					ctx,
					repository.UpdatePaymentTransactionResultParams{
						ID: current.ID,

						Status: current.Status,

						ProviderTransactionID: req.ProviderTransactionID,

						GatewayResponse: nil,
					},
				); err != nil {
					return fmt.Errorf(
						"persist payment transaction provider identity: %w",
						err,
					)
				}

				updated, err =
					transactions.GetByID(
						ctx,
						current.ID,
					)
				if err != nil {
					return fmt.Errorf(
						"reload payment transaction after provider identity update: %w",
						err,
					)
				}

				return nil
			}

			if !canTransition(
				current.Status,
				req.Status,
			) {
				return fmt.Errorf(
					"%w: %s -> %s",
					ErrInvalidTransactionTransition,
					current.Status,
					req.Status,
				)
			}

			if err := transactions.UpdateResult(
				ctx,
				repository.UpdatePaymentTransactionResultParams{
					ID: transactionID,

					Status: req.Status,

					ProviderTransactionID: req.ProviderTransactionID,

					GatewayResponse: req.GatewayResponse,
				},
			); err != nil {
				return fmt.Errorf(
					"persist payment transaction result: %w",
					err,
				)
			}

			updated, err =
				transactions.GetByID(
					ctx,
					transactionID,
				)
			if err != nil {
				return fmt.Errorf(
					"reload payment transaction: %w",
					err,
				)
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return updated, nil
}
