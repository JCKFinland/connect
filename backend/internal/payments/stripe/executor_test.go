package stripe

import (
	"context"
	"errors"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type fakePaymentIntentClient struct{}

func (fakePaymentIntentClient) Create(
	context.Context,
	*stripego.PaymentIntentCreateParams,
) (*stripego.PaymentIntent, error) {
	return nil, errors.New(
		"Stripe provider execution is not implemented",
	)
}

func TestExecutorRejectsInvalidOperation(t *testing.T) {
	t.Parallel()

	idempotencyKey := "idem-test-123"

	validTransaction := func() *models.PaymentTransaction {
		return &models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-123",
			},
			PaymentID: "payment-123",

			Provider: ProviderName,

			IdempotencyKey: &idempotencyKey,

			TransactionType: paymenttransaction.TypeSale,

			Amount:   "23.45",
			Currency: "EUR",
		}
	}

	tests := []struct {
		name string

		transaction *models.PaymentTransaction

		wantError error
	}{
		{
			name:        "nil transaction",
			transaction: nil,
			wantError:   ErrInvalidOperation,
		},
		{
			name: "missing transaction ID",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.ID = ""

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "missing payment ID",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.PaymentID = ""

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "wrong provider",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.Provider = "OTHER_PROVIDER"

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "missing idempotency key",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.IdempotencyKey = nil

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "blank idempotency key",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()

				blank := "   "
				transaction.IdempotencyKey = &blank

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "missing amount",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.Amount = ""

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "missing currency",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.Currency = ""

				return transaction
			}(),
			wantError: ErrInvalidOperation,
		},
		{
			name: "unsupported transaction type",
			transaction: func() *models.PaymentTransaction {
				transaction := validTransaction()
				transaction.TransactionType = "UNKNOWN"

				return transaction
			}(),
			wantError: ErrUnsupportedOperation,
		},
	}

	executor :=
		newExecutorWithPaymentIntents(
			fakePaymentIntentClient{},
		)

	for _, test := range tests {
		test := test

		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				result, err := executor.Execute(
					context.Background(),
					ExecuteRequest{
						Transaction: test.transaction,
					},
				)

				if result != nil {
					t.Fatalf(
						"expected nil result, got %+v",
						result,
					)
				}

				if !errors.Is(
					err,
					test.wantError,
				) {
					t.Fatalf(
						"expected error %v, got %v",
						test.wantError,
						err,
					)
				}
			},
		)
	}
}

func TestExecutorRecognizesSupportedOperationTypes(
	t *testing.T,
) {
	t.Parallel()

	idempotencyKey := "idem-test-123"

	operationTypes := []string{
		paymenttransaction.TypeSale,
		paymenttransaction.TypeAuthorize,
		paymenttransaction.TypeCapture,
		paymenttransaction.TypeRefund,
		paymenttransaction.TypeVoid,
	}

	executor :=
		newExecutorWithPaymentIntents(
			fakePaymentIntentClient{},
		)

	for _, operationType := range operationTypes {
		operationType := operationType

		t.Run(
			operationType,
			func(t *testing.T) {
				t.Parallel()

				transaction := &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: "transaction-123",
					},

					PaymentID: "payment-123",

					Provider: ProviderName,

					IdempotencyKey: &idempotencyKey,

					TransactionType: operationType,

					Amount:   "23.45",
					Currency: "EUR",
				}

				request := ExecuteRequest{
					Transaction: transaction,
				}

				switch operationType {
				case paymenttransaction.TypeCapture,
					paymenttransaction.TypeRefund,
					paymenttransaction.TypeVoid:

					request.ParentProviderTransactionID =
						"pi_parent_test"
				}

				result, err := executor.Execute(
					context.Background(),
					request,
				)

				if result != nil {
					t.Fatalf(
						"expected nil result before provider execution implementation, got %+v",
						result,
					)
				}

				if err == nil {
					t.Fatal(
						"expected not-implemented error",
					)
				}

				if errors.Is(
					err,
					ErrUnsupportedOperation,
				) {
					t.Fatalf(
						"supported operation %s was rejected as unsupported: %v",
						operationType,
						err,
					)
				}

				if errors.Is(
					err,
					ErrInvalidOperation,
				) {
					t.Fatalf(
						"supported operation %s was rejected as invalid: %v",
						operationType,
						err,
					)
				}
			},
		)
	}
}

func TestExecutorRequiresParentProviderIdentityForDependentOperations(
	t *testing.T,
) {
	t.Parallel()

	idempotencyKey := "idem-parent-test"

	tests := []struct {
		name            string
		transactionType string
		parentRequired  bool
	}{
		{
			name:            "sale does not require parent",
			transactionType: paymenttransaction.TypeSale,
			parentRequired:  false,
		},
		{
			name:            "authorize does not require parent",
			transactionType: paymenttransaction.TypeAuthorize,
			parentRequired:  false,
		},
		{
			name:            "capture requires parent",
			transactionType: paymenttransaction.TypeCapture,
			parentRequired:  true,
		},
		{
			name:            "refund requires parent",
			transactionType: paymenttransaction.TypeRefund,
			parentRequired:  true,
		},
		{
			name:            "void requires parent",
			transactionType: paymenttransaction.TypeVoid,
			parentRequired:  true,
		},
	}

	executor :=
		newExecutorWithPaymentIntents(
			fakePaymentIntentClient{},
		)

	for _, tt := range tests {
		tt := tt

		t.Run(
			tt.name,
			func(t *testing.T) {
				t.Parallel()

				transaction := &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: "transaction-123",
					},

					PaymentID: "payment-123",

					Provider: ProviderName,

					IdempotencyKey: &idempotencyKey,

					TransactionType: tt.transactionType,

					Amount: "23.45",

					Currency: "EUR",
				}

				_, err := executor.Execute(
					context.Background(),
					ExecuteRequest{
						Transaction: transaction,
					},
				)

				if tt.parentRequired {
					if !errors.Is(
						err,
						ErrInvalidOperation,
					) {
						t.Fatalf(
							"expected ErrInvalidOperation for missing parent identity, got %v",
							err,
						)
					}

					return
				}

				if errors.Is(
					err,
					ErrInvalidOperation,
				) {
					t.Fatalf(
						"%s unexpectedly required parent identity: %v",
						tt.transactionType,
						err,
					)
				}
			},
		)
	}
}

func TestExecutorAcceptsParentProviderIdentityForDependentOperations(
	t *testing.T,
) {
	t.Parallel()

	idempotencyKey := "idem-parent-positive"

	operationTypes := []string{
		paymenttransaction.TypeCapture,
		paymenttransaction.TypeRefund,
		paymenttransaction.TypeVoid,
	}

	executor :=
		newExecutorWithPaymentIntents(
			fakePaymentIntentClient{},
		)

	for _, operationType := range operationTypes {
		operationType := operationType

		t.Run(
			operationType,
			func(t *testing.T) {
				t.Parallel()

				transaction := &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: "transaction-123",
					},

					PaymentID: "payment-123",

					Provider: ProviderName,

					IdempotencyKey: &idempotencyKey,

					TransactionType: operationType,

					Amount: "23.45",

					Currency: "EUR",
				}

				_, err := executor.Execute(
					context.Background(),
					ExecuteRequest{
						Transaction: transaction,

						ParentProviderTransactionID: "pi_parent_123",
					},
				)

				if errors.Is(
					err,
					ErrInvalidOperation,
				) {
					t.Fatalf(
						"%s rejected valid parent identity: %v",
						operationType,
						err,
					)
				}

				if errors.Is(
					err,
					ErrUnsupportedOperation,
				) {
					t.Fatalf(
						"%s rejected as unsupported: %v",
						operationType,
						err,
					)
				}
			},
		)
	}
}

func TestNewExecutorRequiresSecretKey(
	t *testing.T,
) {
	t.Parallel()

	executor, err :=
		NewExecutor("   ")

	if executor != nil {
		t.Fatalf(
			"expected nil executor, got %+v",
			executor,
		)
	}

	if !errors.Is(
		err,
		ErrInvalidOperation,
	) {
		t.Fatalf(
			"expected ErrInvalidOperation, got %v",
			err,
		)
	}
}

func TestNewExecutorAcceptsSecretKey(
	t *testing.T,
) {
	t.Parallel()

	executor, err :=
		NewExecutor(
			"sk_test_connect",
		)
	if err != nil {
		t.Fatalf(
			"create Stripe executor: %v",
			err,
		)
	}

	if executor == nil {
		t.Fatal(
			"expected Stripe executor",
		)
	}
}

var _ paymentIntentClient = fakePaymentIntentClient{}
