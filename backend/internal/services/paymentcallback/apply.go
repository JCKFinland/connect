package paymentcallback

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	req.Provider = strings.TrimSpace(req.Provider)
	req.TransactionID = strings.TrimSpace(req.TransactionID)
	req.PaymentID = strings.TrimSpace(req.PaymentID)
	req.ProviderTransactionID =
		strings.TrimSpace(req.ProviderTransactionID)
	req.ProviderStatus =
		strings.TrimSpace(req.ProviderStatus)

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

	var (
		transaction *models.PaymentTransaction
		err         error
	)

	// Prefer the provider-authenticated CONNECT transaction identity.
	//
	// Stripe PaymentIntent IDs are provider resource identities and may be
	// reused across AUTHORIZE, CAPTURE, and VOID operations.
	if req.TransactionID != "" {
		transaction, err =
			s.transactions.GetByID(
				ctx,
				req.TransactionID,
			)
	} else {
		// Backward-compatible fallback for providers that do not supply a
		// trusted CONNECT transaction identity.
		transaction, err =
			s.transactions.GetByProviderTransactionID(
				ctx,
				req.Provider,
				req.ProviderTransactionID,
			)
	}

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

	if transaction == nil {
		return nil, ErrCallbackTransactionNotFound
	}

	if !strings.EqualFold(
		strings.TrimSpace(transaction.Provider),
		req.Provider,
	) {
		return nil, ErrCallbackProviderMismatch
	}

	// When the provider supplied the CONNECT payment identity, prove that the
	// resolved operation belongs to exactly that payment.
	if req.PaymentID != "" &&
		transaction.PaymentID != req.PaymentID {

		return nil, ErrInvalidCallback
	}

	// A transaction that already has a provider resource identity may only
	// receive callbacks for that same resource. A missing identity may be
	// populated once by ApplyResult.
	if transaction.ProviderTransactionID != nil {
		existingProviderID :=
			strings.TrimSpace(
				*transaction.ProviderTransactionID,
			)

		if existingProviderID != "" &&
			existingProviderID !=
				req.ProviderTransactionID {

			return nil, ErrInvalidCallback
		}
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
