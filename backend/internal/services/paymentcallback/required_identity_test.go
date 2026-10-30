package paymentcallback

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func TestCallbackRequiresExactConnectIdentities(t *testing.T) {
	tests := []struct {
		name          string
		transactionID string
		paymentID     string
	}{
		{
			name:      "missing transaction identity",
			paymentID: "payment-123",
		},
		{
			name:          "missing payment identity",
			transactionID: "transaction-123",
		},
		{
			name: "missing both identities",
		},
		{
			name:          "whitespace transaction identity",
			transactionID: "   ",
			paymentID:     "payment-123",
		},
		{
			name:          "whitespace payment identity",
			transactionID: "transaction-123",
			paymentID:     "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(Dependencies{
				Transactions:        &fakePaymentTransactionRepository{},
				PaymentTransactions: &fakePaymentTransactionService{},
			})

			_, err := service.ApplyProviderCallback(
				context.Background(),
				ApplyProviderCallbackRequest{
					Provider:              "STRIPE",
					TransactionID:         tt.transactionID,
					PaymentID:             tt.paymentID,
					ProviderTransactionID: "pi_shared_resource",
					ProviderStatus:        paymenttransaction.StatusSuccess,
				},
			)

			if !errors.Is(err, ErrInvalidCallback) {
				t.Fatalf(
					"expected ErrInvalidCallback before repository access, got %v",
					err,
				)
			}
		})
	}
}
