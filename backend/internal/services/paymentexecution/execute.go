package paymentexecution

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type Dependencies struct {
	Transactions repository.PaymentTransactionRepository

	PaymentTransactions paymenttransaction.Service

	Stripe stripepayment.Executor
}

type service struct {
	transactions repository.PaymentTransactionRepository

	paymentTransactions paymenttransaction.Service

	stripe stripepayment.Executor
}

func NewService(
	deps Dependencies,
) Service {
	return &service{
		transactions: deps.Transactions,

		paymentTransactions: deps.PaymentTransactions,

		stripe: deps.Stripe,
	}
}

func (s *service) Execute(
	ctx context.Context,
	transactionID string,
) (*models.PaymentTransaction, error) {
	if transactionID == "" {
		return nil, errors.New(
			"payment transaction ID is required",
		)
	}

	if s.transactions == nil {
		return nil, errors.New(
			"payment transaction repository is not configured",
		)
	}

	if s.paymentTransactions == nil {
		return nil, errors.New(
			"payment transaction service is not configured",
		)
	}

	transaction, err :=
		s.transactions.GetByID(
			ctx,
			transactionID,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"get payment transaction for execution: %w",
			err,
		)
	}

	switch transaction.Provider {
	case stripepayment.ProviderName:
		return s.executeStripe(
			ctx,
			transaction,
		)

	default:
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedProvider,
			transaction.Provider,
		)
	}
}

func (s *service) executeStripe(
	ctx context.Context,
	transaction *models.PaymentTransaction,
) (*models.PaymentTransaction, error) {
	if s.stripe == nil {
		return nil, errors.New(
			"Stripe payment executor is not configured",
		)
	}

	parentProviderTransactionID, err :=
		s.resolveParentProviderTransactionID(
			ctx,
			transaction,
		)
	if err != nil {
		return nil, err
	}

	result, err :=
		s.stripe.Execute(
			ctx,
			stripepayment.ExecuteRequest{
				Transaction: transaction,

				ParentProviderTransactionID: parentProviderTransactionID,
			},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"execute Stripe payment operation: %w",
			err,
		)
	}

	if result == nil {
		return nil, errors.New(
			"Stripe executor returned nil result",
		)
	}

	providerTransactionID :=
		result.ProviderTransactionID

	updated, err :=
		s.paymentTransactions.ApplyResult(
			ctx,
			transaction.ID,
			paymenttransaction.ApplyResultRequest{
				Status: result.Status,

				ProviderTransactionID: &providerTransactionID,
			},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"apply Stripe execution result: %w",
			err,
		)
	}

	return updated, nil
}

func (s *service) resolveParentProviderTransactionID(
	ctx context.Context,
	transaction *models.PaymentTransaction,
) (string, error) {
	switch transaction.TransactionType {

	case paymenttransaction.TypeSale,
		paymenttransaction.TypeAuthorize:

		return "", nil

	case paymenttransaction.TypeCapture,
		paymenttransaction.TypeVoid:

		parent, err :=
			s.transactions.GetLatestSuccessfulByPaymentAndTypes(
				ctx,
				transaction.PaymentID,
				transaction.Provider,
				[]string{
					paymenttransaction.TypeAuthorize,
				},
			)
		if err != nil {
			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return "",
					ErrParentProviderTransactionNotFound
			}

			return "", fmt.Errorf(
				"resolve authorization parent: %w",
				err,
			)
		}

		return requireParentProviderIdentity(
			parent,
		)

	case paymenttransaction.TypeRefund:

		parent, err :=
			s.transactions.GetLatestSuccessfulByPaymentAndTypes(
				ctx,
				transaction.PaymentID,
				transaction.Provider,
				[]string{
					paymenttransaction.TypeSale,
					paymenttransaction.TypeCapture,
				},
			)
		if err != nil {
			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return "",
					ErrParentProviderTransactionNotFound
			}

			return "", fmt.Errorf(
				"resolve refund parent: %w",
				err,
			)
		}

		return requireParentProviderIdentity(
			parent,
		)

	default:
		return "", fmt.Errorf(
			"unsupported transaction type: %s",
			transaction.TransactionType,
		)
	}
}

func requireParentProviderIdentity(
	parent *models.PaymentTransaction,
) (string, error) {
	if parent == nil ||
		parent.ProviderTransactionID == nil ||
		*parent.ProviderTransactionID == "" {

		return "",
			ErrParentProviderTransactionNotFound
	}

	return *parent.ProviderTransactionID,
		nil
}

var _ Service = (*service)(nil)
