package paymentcallback

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func TestCallbackExactTransactionAttribution(t *testing.T) {
	providerID := "pi_shared_provider_resource"

	tests := []struct {
		name             string
		paymentID        string
		provider         string
		storedProviderID string
		wantErr          error
		wantApply        bool
	}{
		{
			name:             "exact transaction accepted",
			paymentID:        "payment-owner",
			provider:         "STRIPE",
			storedProviderID: providerID,
			wantApply:        true,
		},
		{
			name:             "foreign payment rejected",
			paymentID:        "payment-foreign",
			provider:         "STRIPE",
			storedProviderID: providerID,
			wantErr:          ErrInvalidCallback,
		},
		{
			name:             "foreign provider rejected",
			paymentID:        "payment-owner",
			provider:         "OTHER",
			storedProviderID: providerID,
			wantErr:          ErrCallbackProviderMismatch,
		},
		{
			name:             "different provider resource rejected",
			paymentID:        "payment-owner",
			provider:         "STRIPE",
			storedProviderID: "pi_other_resource",
			wantErr:          ErrInvalidCallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exactLookupCalled := false
			fallbackCalled := false
			applyCalled := false

			repo := &fakePaymentTransactionRepository{
				getByID: func(
					_ context.Context,
					id string,
				) (*models.PaymentTransaction, error) {
					exactLookupCalled = true

					if id != "transaction-owner" {
						t.Fatalf("unexpected transaction lookup: %q", id)
					}

					return &models.PaymentTransaction{
						BaseModel: models.BaseModel{
							ID: "transaction-owner",
						},
						PaymentID:             "payment-owner",
						Provider:              "STRIPE",
						ProviderTransactionID: &tt.storedProviderID,
					}, nil
				},
				getByProviderTransactionID: func(
					_ context.Context,
					_ string,
					_ string,
				) (*models.PaymentTransaction, error) {
					fallbackCalled = true

					return &models.PaymentTransaction{
						BaseModel: models.BaseModel{
							ID: "transaction-foreign",
						},
						PaymentID: "payment-foreign",
						Provider:  "STRIPE",
					}, nil
				},
			}

			service := NewService(Dependencies{
				Transactions: repo,
				PaymentTransactions: &fakePaymentTransactionService{
					applyResult: func(
						_ context.Context,
						transactionID string,
						_ paymenttransaction.ApplyResultRequest,
					) (*models.PaymentTransaction, error) {
						applyCalled = true

						if transactionID != "transaction-owner" {
							t.Fatalf(
								"callback applied to wrong transaction: %q",
								transactionID,
							)
						}

						return &models.PaymentTransaction{
							BaseModel: models.BaseModel{
								ID: transactionID,
							},
							PaymentID: "payment-owner",
							Provider:  "STRIPE",
						}, nil
					},
				},
			})

			_, err := service.ApplyProviderCallback(
				context.Background(),
				ApplyProviderCallbackRequest{
					Provider:              tt.provider,
					TransactionID:         "transaction-owner",
					PaymentID:             tt.paymentID,
					ProviderTransactionID: providerID,
					ProviderStatus:        "SUCCESS",
				},
			)

			if !exactLookupCalled {
				t.Fatal("exact CONNECT transaction lookup was not executed")
			}

			if fallbackCalled {
				t.Fatal("provider-resource fallback used despite exact identity")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("unexpected error: got %v, want %v", err, tt.wantErr)
			}

			if applyCalled != tt.wantApply {
				t.Fatalf(
					"unexpected ApplyResult invocation: got %v, want %v",
					applyCalled,
					tt.wantApply,
				)
			}
		})
	}
}
