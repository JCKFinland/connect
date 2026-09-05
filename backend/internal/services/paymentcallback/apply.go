package paymentcallback

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type Dependencies struct {
	Transactions repository.PaymentTransactionRepository

	PaymentTransactions paymenttransaction.Service
}

type paymentCallbackService struct {
	transactions repository.PaymentTransactionRepository

	paymentTransactions paymenttransaction.Service
}

func NewService(
	deps Dependencies,
) Service {
	return &paymentCallbackService{
		transactions:        deps.Transactions,
		paymentTransactions: deps.PaymentTransactions,
	}
}

func (s *paymentCallbackService) ApplyProviderCallback(
	ctx context.Context,
	req ApplyProviderCallbackRequest,
) (*models.PaymentTransaction, error) {
	if req.Provider == "" {
		return nil, fmt.Errorf(
			"%w: provider is required",
			ErrInvalidCallback,
		)
	}

	if req.ProviderTransactionID == "" {
		return nil, fmt.Errorf(
			"%w: provider transaction ID is required",
			ErrInvalidCallback,
		)
	}

	if req.ProviderStatus == "" {
		return nil, fmt.Errorf(
			"%w: provider status is required",
			ErrInvalidCallback,
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
		s.transactions.GetByProviderTransactionID(
			ctx,
			req.Provider,
			req.ProviderTransactionID,
		)

	if errors.Is(
		err,
		repository.ErrNotFound,
	) {
		return nil, ErrCallbackTransactionNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"resolve payment transaction for callback: %w",
			err,
		)
	}

	if transaction.Provider != req.Provider {
		return nil, ErrCallbackProviderMismatch
	}

	result, err :=
		s.paymentTransactions.ApplyResult(
			ctx,
			transaction.ID,
			paymenttransaction.ApplyResultRequest{
				Status: req.ProviderStatus,

				ProviderTransactionID: &req.ProviderTransactionID,

				GatewayResponse: req.RawPayload,
			},
		)
	if err != nil {
		return nil, fmt.Errorf(
			"apply verified payment provider callback: %w",
			err,
		)
	}

	return result, nil
}

var _ Service = (*paymentCallbackService)(nil)
