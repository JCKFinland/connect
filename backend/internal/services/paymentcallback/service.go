package paymentcallback

import (
	"context"
	"encoding/json"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type ApplyProviderCallbackRequest struct {
	Provider string

	// ProviderTransactionID identifies the provider-side financial
	// operation being reported by the callback.
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
