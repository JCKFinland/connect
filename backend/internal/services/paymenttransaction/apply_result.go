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

			payments :=
				postgresrepo.NewPaymentRepositoryWithDB(
					tx,
				)

			// Read the transaction first so we know which parent
			// payment row must be locked.
			candidate, err :=
				transactions.GetByID(
					ctx,
					transactionID,
				)
			if err != nil {
				return fmt.Errorf(
					"get payment transaction before reconciliation lock: %w",
					err,
				)
			}

			// Lock the aggregate payment before the child transaction.
			//
			// This serializes all financial operations belonging to
			// the same payment, including concurrent refunds.
			currentPayment, err :=
				payments.GetByIDForUpdate(
					ctx,
					candidate.PaymentID,
				)
			if err != nil {
				return fmt.Errorf(
					"lock aggregate payment for reconciliation: %w",
					err,
				)
			}

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

			// A provider identity may be filled once, but never changed.
			if current.ProviderTransactionID != nil &&
				req.ProviderTransactionID != nil &&
				*current.ProviderTransactionID !=
					*req.ProviderTransactionID {

				return ErrProviderIdentityConflict
			}

			// ---------------------------------------------------------
			// Same-status provider delivery.
			// ---------------------------------------------------------

			if current.Status == req.Status {
				// A later duplicate delivery may legitimately provide
				// the provider transaction ID that was unavailable on
				// the first delivery.
				if current.ProviderTransactionID == nil &&
					req.ProviderTransactionID != nil {

					if err := transactions.UpdateResult(
						ctx,
						repository.UpdatePaymentTransactionResultParams{
							ID: current.ID,

							Status: current.Status,

							ProviderTransactionID: req.ProviderTransactionID,

							// Do not overwrite the original provider
							// response during identity enrichment.
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
				} else {
					updated = current
				}

				// A SUCCESS replay must still reconcile the aggregate
				// payment. This makes reconciliation repairable if an
				// earlier attempt updated the provider transaction but
				// did not complete aggregate state reconciliation.
				if updated.Status == StatusSuccess {
					if err := reconcileSuccessfulPaymentOperation(
						ctx,
						payments,
						transactions,
						currentPayment,
						updated,
					); err != nil {
						return fmt.Errorf(
							"reconcile successful payment operation: %w",
							err,
						)
					}
				}

				return nil
			}

			// ---------------------------------------------------------
			// New lifecycle transition.
			// ---------------------------------------------------------

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

			// Successful provider operations now reconcile the
			// authoritative aggregate payment inside this same
			// PostgreSQL transaction.
			if updated.Status == StatusSuccess {
				if err := reconcileSuccessfulPaymentOperation(
					ctx,
					payments,
					transactions,
					currentPayment,
					updated,
				); err != nil {
					return fmt.Errorf(
						"reconcile successful payment operation: %w",
						err,
					)
				}
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return updated, nil
}
