package paymentcallback

import (
	"context"
	"encoding/json"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type ApplyProviderCallbackRequest struct {
	Provider string

	// TransactionID is the exact CONNECT transaction identity extracted from
	// trusted provider callback metadata when available.
	TransactionID string

	// PaymentID is the CONNECT payment identity extracted from trusted
	// provider callback metadata when available.
	PaymentID string

	// ProviderTransactionID identifies the provider-side payment resource.
	ProviderTransactionID string

	// ProviderStatus is the provider-neutral lifecycle result after
	// verification/mapping by the provider adapter.
	ProviderStatus string

	// RawPayload preserves the verified provider callback body for
	// audit/reconciliation evidence.
	RawPayload json.RawMessage
}

type Service interface {
	ApplyProviderCallback(
		ctx context.Context,
		req ApplyProviderCallbackRequest,
	) (*models.PaymentTransaction, error)
}
