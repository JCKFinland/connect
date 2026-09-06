package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

var (
	ErrInvalidOperation = errors.New(
		"invalid Stripe payment operation",
	)

	ErrUnsupportedOperation = errors.New(
		"unsupported Stripe payment operation",
	)
)

// ExecuteRequest contains the already-persisted CONNECT financial
// operation that must be submitted to Stripe.
//
// The PaymentTransaction remains authoritative for amount, currency,
// transaction type, reference, and idempotency identity.
type ExecuteRequest struct {
	Transaction *models.PaymentTransaction

	// ParentProviderTransactionID is the previously persisted Stripe
	// provider identity required by operations that act on an existing
	// PaymentIntent.
	//
	// Required for:
	//   CAPTURE
	//   REFUND
	//   VOID
	//
	// Not required for:
	//   SALE
	//   AUTHORIZE
	ParentProviderTransactionID string
}

// ExecuteResult represents the immediate provider-side state returned
// after submitting an operation to Stripe.
//
// Final asynchronous state may subsequently arrive through the
// verified Stripe webhook boundary.
type ExecuteResult struct {
	ProviderTransactionID string

	Status string
}

type Executor interface {
	Execute(
		ctx context.Context,
		req ExecuteRequest,
	) (*ExecuteResult, error)
}

type executor struct{}

func NewExecutor() Executor {
	return &executor{}
}

func (e *executor) Execute(
	ctx context.Context,
	req ExecuteRequest,
) (*ExecuteResult, error) {
	if req.Transaction == nil {
		return nil, fmt.Errorf(
			"%w: payment transaction is required",
			ErrInvalidOperation,
		)
	}

	transaction := req.Transaction

	if strings.TrimSpace(transaction.ID) == "" {
		return nil, fmt.Errorf(
			"%w: transaction ID is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(transaction.PaymentID) == "" {
		return nil, fmt.Errorf(
			"%w: payment ID is required",
			ErrInvalidOperation,
		)
	}

	if !strings.EqualFold(
		strings.TrimSpace(transaction.Provider),
		ProviderName,
	) {
		return nil, fmt.Errorf(
			"%w: provider must be %s",
			ErrInvalidOperation,
			ProviderName,
		)
	}

	if transaction.IdempotencyKey == nil ||
		strings.TrimSpace(*transaction.IdempotencyKey) == "" {

		return nil, fmt.Errorf(
			"%w: idempotency key is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(transaction.Amount) == "" {
		return nil, fmt.Errorf(
			"%w: amount is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(transaction.Currency) == "" {
		return nil, fmt.Errorf(
			"%w: currency is required",
			ErrInvalidOperation,
		)
	}

	switch transaction.TransactionType {
	case paymenttransaction.TypeSale,
		paymenttransaction.TypeAuthorize,
		paymenttransaction.TypeCapture,
		paymenttransaction.TypeRefund,
		paymenttransaction.TypeVoid:

		switch transaction.TransactionType {
		case paymenttransaction.TypeCapture,
			paymenttransaction.TypeRefund,
			paymenttransaction.TypeVoid:

			if strings.TrimSpace(
				req.ParentProviderTransactionID,
			) == "" {
				return nil, fmt.Errorf(
					"%w: parent provider transaction ID is required for %s",
					ErrInvalidOperation,
					transaction.TransactionType,
				)
			}
		}

		// Supported CONNECT operation types. Provider execution will
		// be implemented behind this boundary.

	default:
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedOperation,
			transaction.TransactionType,
		)
	}

	return nil, errors.New(
		"Stripe provider execution is not implemented",
	)

}

var _ Executor = (*executor)(nil)
