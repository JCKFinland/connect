package paymentcallback

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type fakePaymentTransactionRepository struct {
	getByProviderTransactionID func(
		ctx context.Context,
		provider string,
		providerTransactionID string,
	) (*models.PaymentTransaction, error)
}

func (f *fakePaymentTransactionRepository) Create(
	context.Context,
	repository.CreatePaymentTransactionParams,
) (*models.PaymentTransaction, error) {
	panic("unexpected Create call")
}

func (f *fakePaymentTransactionRepository) GetByID(
	context.Context,
	string,
) (*models.PaymentTransaction, error) {
	panic("unexpected GetByID call")
}

func (f *fakePaymentTransactionRepository) GetByReference(
	context.Context,
	string,
) (*models.PaymentTransaction, error) {
	panic("unexpected GetByReference call")
}

func (f *fakePaymentTransactionRepository) GetByProviderIdempotencyKey(
	context.Context,
	string,
	string,
) (*models.PaymentTransaction, error) {
	panic("unexpected GetByProviderIdempotencyKey call")
}

func (f *fakePaymentTransactionRepository) GetByProviderTransactionID(
	ctx context.Context,
	provider string,
	providerTransactionID string,
) (*models.PaymentTransaction, error) {
	if f.getByProviderTransactionID == nil {
		panic("unexpected GetByProviderTransactionID call")
	}

	return f.getByProviderTransactionID(
		ctx,
		provider,
		providerTransactionID,
	)
}

func (f *fakePaymentTransactionRepository) GetByIDForUpdate(
	context.Context,
	string,
) (*models.PaymentTransaction, error) {
	panic("unexpected GetByIDForUpdate call")
}

func (f *fakePaymentTransactionRepository) UpdateResult(
	context.Context,
	repository.UpdatePaymentTransactionResultParams,
) error {
	panic("unexpected UpdateResult call")
}

func (f *fakePaymentTransactionRepository) GetSuccessfulRefundState(
	context.Context,
	string,
) (repository.SuccessfulRefundState, error) {
	panic("unexpected GetSuccessfulRefundState call")
}

func (f *fakePaymentTransactionRepository) ValidateRefundAmount(
	context.Context,
	string,
	string,
) error {
	panic("unexpected ValidateRefundAmount call")
}

type fakePaymentTransactionService struct {
	applyResult func(
		ctx context.Context,
		transactionID string,
		req paymenttransaction.ApplyResultRequest,
	) (*models.PaymentTransaction, error)
}

func (f *fakePaymentTransactionService) InitiateOperation(
	context.Context,
	paymenttransaction.InitiateOperationRequest,
) (*models.PaymentTransaction, error) {
	panic("unexpected InitiateOperation call")
}

func (f *fakePaymentTransactionService) ApplyResult(
	ctx context.Context,
	transactionID string,
	req paymenttransaction.ApplyResultRequest,
) (*models.PaymentTransaction, error) {
	if f.applyResult == nil {
		panic("unexpected ApplyResult call")
	}

	return f.applyResult(
		ctx,
		transactionID,
		req,
	)
}

func TestApplyProviderCallbackForwardsVerifiedResult(
	t *testing.T,
) {
	rawPayload :=
		json.RawMessage(`{"status":"captured"}`)

	repo :=
		&fakePaymentTransactionRepository{
			getByProviderTransactionID: func(
				_ context.Context,
				provider string,
				providerTransactionID string,
			) (*models.PaymentTransaction, error) {
				if provider != "TEST_PROVIDER" {
					t.Fatalf(
						"unexpected provider %q",
						provider,
					)
				}

				if providerTransactionID != "provider-123" {
					t.Fatalf(
						"unexpected provider transaction ID %q",
						providerTransactionID,
					)
				}

				return &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: "transaction-123",
					},

					Provider: provider,

					ProviderTransactionID: &providerTransactionID,
				}, nil
			},
		}

	transactionService :=
		&fakePaymentTransactionService{
			applyResult: func(
				_ context.Context,
				transactionID string,
				req paymenttransaction.ApplyResultRequest,
			) (*models.PaymentTransaction, error) {
				if transactionID == "" {
					t.Fatal(
						"expected CONNECT transaction ID",
					)
				}

				if req.Status != paymenttransaction.StatusSuccess {
					t.Fatalf(
						"unexpected status %q",
						req.Status,
					)
				}

				if req.ProviderTransactionID == nil ||
					*req.ProviderTransactionID != "provider-123" {

					t.Fatal(
						"provider transaction identity was not preserved",
					)
				}

				if string(req.GatewayResponse) != string(rawPayload) {
					t.Fatalf(
						"gateway response mismatch: got %s want %s",
						string(req.GatewayResponse),
						string(rawPayload),
					)
				}

				return &models.PaymentTransaction{
					Provider: "TEST_PROVIDER",
					Status:   paymenttransaction.StatusSuccess,
				}, nil
			},
		}

	service := NewService(
		Dependencies{
			Transactions: repo,

			PaymentTransactions: transactionService,
		},
	)

	result, err :=
		service.ApplyProviderCallback(
			context.Background(),
			ApplyProviderCallbackRequest{
				Provider: "TEST_PROVIDER",

				ProviderTransactionID: "provider-123",

				ProviderStatus: paymenttransaction.StatusSuccess,

				RawPayload: rawPayload,
			},
		)
	if err != nil {
		t.Fatalf(
			"apply callback: %v",
			err,
		)
	}

	if result.Status != paymenttransaction.StatusSuccess {
		t.Fatalf(
			"expected SUCCESS, got %s",
			result.Status,
		)
	}
}

func TestApplyProviderCallbackNotFound(
	t *testing.T,
) {
	service := NewService(
		Dependencies{
			Transactions: &fakePaymentTransactionRepository{
				getByProviderTransactionID: func(
					context.Context,
					string,
					string,
				) (*models.PaymentTransaction, error) {
					return nil, repository.ErrNotFound
				},
			},

			PaymentTransactions: &fakePaymentTransactionService{},
		},
	)

	_, err :=
		service.ApplyProviderCallback(
			context.Background(),
			ApplyProviderCallbackRequest{
				Provider: "TEST_PROVIDER",

				ProviderTransactionID: "missing-provider-id",

				ProviderStatus: paymenttransaction.StatusSuccess,
			},
		)

	if !errors.Is(
		err,
		ErrCallbackTransactionNotFound,
	) {
		t.Fatalf(
			"expected ErrCallbackTransactionNotFound, got %v",
			err,
		)
	}
}

func TestApplyProviderCallbackRejectsInvalidRequest(
	t *testing.T,
) {
	service := NewService(
		Dependencies{
			Transactions: &fakePaymentTransactionRepository{},

			PaymentTransactions: &fakePaymentTransactionService{},
		},
	)

	tests := []struct {
		name string
		req  ApplyProviderCallbackRequest
	}{
		{
			name: "missing provider",
			req: ApplyProviderCallbackRequest{
				ProviderTransactionID: "provider-123",

				ProviderStatus: paymenttransaction.StatusSuccess,
			},
		},
		{
			name: "missing provider transaction ID",
			req: ApplyProviderCallbackRequest{
				Provider: "TEST_PROVIDER",

				ProviderStatus: paymenttransaction.StatusSuccess,
			},
		},
		{
			name: "missing provider status",
			req: ApplyProviderCallbackRequest{
				Provider: "TEST_PROVIDER",

				ProviderTransactionID: "provider-123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err :=
					service.ApplyProviderCallback(
						context.Background(),
						tt.req,
					)

				if !errors.Is(
					err,
					ErrInvalidCallback,
				) {
					t.Fatalf(
						"expected ErrInvalidCallback, got %v",
						err,
					)
				}
			},
		)
	}
}
