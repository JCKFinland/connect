package repository

import (
	"context"
	"encoding/json"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type CreatePaymentTransactionParams struct {
	PaymentID string

	TransactionReference string

	Provider string

	IdempotencyKey *string

	TransactionType string

	Amount   string
	Currency string

	GatewayRequest json.RawMessage
}

// PaymentTransactionRepository defines persistence operations for
// provider-facing payment transactions.
type PaymentTransactionRepository interface {
	Create(
		ctx context.Context,
		params CreatePaymentTransactionParams,
	) (*models.PaymentTransaction, error)

	GetByID(
		ctx context.Context,
		id string,
	) (*models.PaymentTransaction, error)

	GetByReference(
		ctx context.Context,
		transactionReference string,
	) (*models.PaymentTransaction, error)

	GetByProviderIdempotencyKey(
		ctx context.Context,
		provider string,
		idempotencyKey string,
	) (*models.PaymentTransaction, error)

	GetByProviderTransactionID(
		ctx context.Context,
		provider string,
		providerTransactionID string,
	) (*models.PaymentTransaction, error)

	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.PaymentTransaction, error)
}
