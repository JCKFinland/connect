package paymenttransaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func (s *paymentTransactionService) InitiateOperation(
	ctx context.Context,
	req InitiateOperationRequest,
) (*models.PaymentTransaction, error) {
	if req.PaymentID == "" {
		return nil, errors.New(
			"payment ID is required",
		)
	}

	if req.Provider == "" {
		return nil, errors.New(
			"payment provider is required",
		)
	}

	if req.IdempotencyKey == "" {
		return nil, errors.New(
			"idempotency key is required",
		)
	}

	if !isValidTransactionType(
		req.TransactionType,
	) {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedTransactionType,
			req.TransactionType,
		)
	}

	if req.TransactionType == TypeRefund &&
		req.Amount == "" {

		return nil, ErrPaymentOperationAmountRequired
	}

	if s.db == nil {
		return nil, errors.New(
			"payment transaction database is not configured",
		)
	}

	var result *models.PaymentTransaction

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			payments :=
				postgresrepo.NewPaymentRepositoryWithDB(
					tx,
				)

			transactions :=
				postgresrepo.NewPaymentTransactionRepositoryWithDB(
					tx,
				)

			// Serialize all financial operations belonging to the
			// same logical payment.
			currentPayment, err :=
				payments.GetByIDForUpdate(
					ctx,
					req.PaymentID,
				)
			if err != nil {
				return fmt.Errorf(
					"lock payment for operation initiation: %w",
					err,
				)
			}

			// Serialize provider/idempotency identity globally across
			// all CONNECT backend instances.
			lockKey := fmt.Sprintf(
				"payment-operation:%s:%s",
				req.Provider,
				req.IdempotencyKey,
			)

			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				lockKey,
			); err != nil {
				return fmt.Errorf(
					"lock payment operation identity: %w",
					err,
				)
			}

			existing, err :=
				transactions.GetByProviderIdempotencyKey(
					ctx,
					req.Provider,
					req.IdempotencyKey,
				)

			if err == nil {
				expectedAmount :=
					currentPayment.Amount

				if req.TransactionType == TypeRefund {
					expectedAmount = req.Amount
				}

				if existing.PaymentID != req.PaymentID ||
					existing.TransactionType != req.TransactionType ||
					existing.Amount != expectedAmount {

					return ErrPaymentOperationIdempotencyConflict
				}

				result = existing
				return nil
			}

			if !errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return fmt.Errorf(
					"check existing payment operation: %w",
					err,
				)
			}

			if !canInitiateOperation(
				currentPayment.Status,
				req.TransactionType,
			) {
				return fmt.Errorf(
					"%w: payment status %s cannot initiate %s",
					ErrInvalidPaymentOperation,
					currentPayment.Status,
					req.TransactionType,
				)
			}

			amount := currentPayment.Amount

			if req.TransactionType == TypeRefund {
				if err := transactions.ValidateRefundAmount(
					ctx,
					currentPayment.ID,
					req.Amount,
				); err != nil {
					return fmt.Errorf(
						"%w: %v",
						ErrPaymentOperationAmountInvalid,
						err,
					)
				}

				amount = req.Amount
			}

			idempotencyKey :=
				req.IdempotencyKey

			result, err =
				transactions.Create(
					ctx,
					repository.CreatePaymentTransactionParams{
						PaymentID: currentPayment.ID,

						TransactionReference: "txn_" + uuid.NewString(),

						Provider: req.Provider,

						IdempotencyKey: &idempotencyKey,

						TransactionType: req.TransactionType,

						Amount: amount,

						Currency: currentPayment.Currency,

						GatewayRequest: req.GatewayRequest,
					},
				)
			if err != nil {
				return fmt.Errorf(
					"create payment operation: %w",
					err,
				)
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}
