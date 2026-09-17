package paymentexecution

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type fakePaymentTransactionRepository struct {
	getByID func(
		ctx context.Context,
		id string,
	) (*models.PaymentTransaction, error)

	getLatestSuccessfulByPaymentAndTypes func(
		ctx context.Context,
		paymentID string,
		provider string,
		transactionTypes []string,
	) (*models.PaymentTransaction, error)
}

func (f *fakePaymentTransactionRepository) Create(
	context.Context,
	repository.CreatePaymentTransactionParams,
) (*models.PaymentTransaction, error) {
	panic("unexpected Create call")
}

func (f *fakePaymentTransactionRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.PaymentTransaction, error) {
	if f.getByID == nil {
		panic("unexpected GetByID call")
	}

	return f.getByID(
		ctx,
		id,
	)
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
	context.Context,
	string,
	string,
) (*models.PaymentTransaction, error) {
	panic("unexpected GetByProviderTransactionID call")
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

func (f *fakePaymentTransactionRepository) GetLatestSuccessfulByPaymentAndTypes(
	ctx context.Context,
	paymentID string,
	provider string,
	transactionTypes []string,
) (*models.PaymentTransaction, error) {
	if f.getLatestSuccessfulByPaymentAndTypes == nil {
		panic(
			"unexpected GetLatestSuccessfulByPaymentAndTypes call",
		)
	}

	return f.getLatestSuccessfulByPaymentAndTypes(
		ctx,
		paymentID,
		provider,
		transactionTypes,
	)
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

type fakeStripeExecutor struct {
	execute func(
		ctx context.Context,
		req stripepayment.ExecuteRequest,
	) (*stripepayment.ExecuteResult, error)
}

func (f *fakeStripeExecutor) Execute(
	ctx context.Context,
	req stripepayment.ExecuteRequest,
) (*stripepayment.ExecuteResult, error) {
	if f.execute == nil {
		panic("unexpected Stripe Execute call")
	}

	return f.execute(
		ctx,
		req,
	)
}

func TestExecuteStripeSaleDoesNotResolveParent(
	t *testing.T,
) {
	transaction :=
		testExecutionTransaction(
			paymenttransaction.TypeSale,
		)

	repo :=
		&fakePaymentTransactionRepository{
			getByID: func(
				_ context.Context,
				id string,
			) (*models.PaymentTransaction, error) {
				if id != transaction.ID {
					t.Fatalf(
						"unexpected transaction ID %q",
						id,
					)
				}

				return transaction, nil
			},

			getLatestSuccessfulByPaymentAndTypes: func(
				context.Context,
				string,
				string,
				[]string,
			) (*models.PaymentTransaction, error) {
				t.Fatal(
					"SALE must not resolve a parent provider transaction",
				)

				return nil, nil
			},
		}

	stripeExecutor :=
		&fakeStripeExecutor{
			execute: func(
				_ context.Context,
				req stripepayment.ExecuteRequest,
			) (*stripepayment.ExecuteResult, error) {
				if req.Transaction.ID !=
					transaction.ID {

					t.Fatalf(
						"unexpected transaction %s",
						req.Transaction.ID,
					)
				}

				if req.ParentProviderTransactionID != "" {
					t.Fatalf(
						"SALE unexpectedly received parent identity %q",
						req.ParentProviderTransactionID,
					)
				}

				return &stripepayment.ExecuteResult{
					ProviderTransactionID: "pi_sale_123",

					Status: paymenttransaction.StatusProcessing,

					ClientSecret: "pi_sale_123_secret_test",

					RequiresCustomerAction: true,
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
				if transactionID != transaction.ID {
					t.Fatalf(
						"unexpected ApplyResult transaction ID %q",
						transactionID,
					)
				}

				if req.ProviderTransactionID == nil ||
					*req.ProviderTransactionID !=
						"pi_sale_123" {

					t.Fatal(
						"Stripe provider identity was not forwarded",
					)
				}

				if req.Status !=
					paymenttransaction.StatusProcessing {

					t.Fatalf(
						"unexpected result status %q",
						req.Status,
					)
				}

				return &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: transaction.ID,
					},

					Status: req.Status,

					ProviderTransactionID: req.ProviderTransactionID,
				}, nil
			},
		}

	service :=
		NewService(
			Dependencies{
				Transactions: repo,

				PaymentTransactions: transactionService,

				Stripe: stripeExecutor,
			},
		)

	result, err :=
		service.Execute(
			context.Background(),
			transaction.ID,
		)
	if err != nil {
		t.Fatalf(
			"execute Stripe SALE: %v",
			err,
		)
	}

	if result == nil {
		t.Fatal(
			"expected payment execution result",
		)
	}

	if result.Transaction == nil {
		t.Fatal(
			"expected payment transaction result",
		)
	}

	if result.Transaction.Status !=
		paymenttransaction.StatusProcessing {

		t.Fatalf(
			"expected PROCESSING, got %s",
			result.Transaction.Status,
		)
	}

	if result.ClientSecret !=
		"pi_sale_123_secret_test" {

		t.Fatalf(
			"client secret mismatch: got %q",
			result.ClientSecret,
		)
	}

	if !result.RequiresCustomerAction {
		t.Fatal(
			"expected customer action to be required",
		)
	}
}

func TestExecuteStripeSaleSuccessRemainsWebhookAuthoritative(
	t *testing.T,
) {
	transaction :=
		testExecutionTransaction(
			paymenttransaction.TypeSale,
		)

	repo :=
		&fakePaymentTransactionRepository{
			getByID: func(
				_ context.Context,
				id string,
			) (*models.PaymentTransaction, error) {
				if id != transaction.ID {
					t.Fatalf(
						"unexpected transaction ID %q",
						id,
					)
				}

				return transaction, nil
			},

			getLatestSuccessfulByPaymentAndTypes: func(
				context.Context,
				string,
				string,
				[]string,
			) (*models.PaymentTransaction, error) {
				t.Fatal(
					"SALE must not resolve a parent provider transaction",
				)

				return nil, nil
			},
		}

	stripeExecutor :=
		&fakeStripeExecutor{
			execute: func(
				_ context.Context,
				req stripepayment.ExecuteRequest,
			) (*stripepayment.ExecuteResult, error) {
				return &stripepayment.ExecuteResult{
					ProviderTransactionID: "pi_sale_succeeded",

					Status: paymenttransaction.StatusSuccess,

					ClientSecret: "pi_sale_succeeded_secret_test",

					RequiresCustomerAction: false,
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
				if transactionID != transaction.ID {
					t.Fatalf(
						"unexpected ApplyResult transaction ID %q",
						transactionID,
					)
				}

				if req.ProviderTransactionID == nil ||
					*req.ProviderTransactionID !=
						"pi_sale_succeeded" {

					t.Fatal(
						"Stripe provider identity was not forwarded",
					)
				}

				if req.Status !=
					paymenttransaction.StatusProcessing {

					t.Fatalf(
						"SALE execution must remain PROCESSING until webhook, got %s",
						req.Status,
					)
				}

				return &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: transaction.ID,
					},

					Status: req.Status,

					ProviderTransactionID: req.ProviderTransactionID,
				}, nil
			},
		}

	service :=
		NewService(
			Dependencies{
				Transactions: repo,

				PaymentTransactions: transactionService,

				Stripe: stripeExecutor,
			},
		)

	result, err :=
		service.Execute(
			context.Background(),
			transaction.ID,
		)
	if err != nil {
		t.Fatalf(
			"execute recovered Stripe SALE: %v",
			err,
		)
	}

	if result.Transaction.Status !=
		paymenttransaction.StatusProcessing {

		t.Fatalf(
			"expected webhook-authoritative PROCESSING, got %s",
			result.Transaction.Status,
		)
	}
}

func TestExecuteStripeAuthorizeDoesNotResolveParent(
	t *testing.T,
) {
	assertExecutionWithoutParent(
		t,
		paymenttransaction.TypeAuthorize,
	)
}

func TestExecuteStripeCaptureResolvesAuthorizeParent(
	t *testing.T,
) {
	assertExecutionWithParent(
		t,
		paymenttransaction.TypeCapture,
		[]string{
			paymenttransaction.TypeAuthorize,
		},
	)
}

func TestExecuteStripeVoidResolvesAuthorizeParent(
	t *testing.T,
) {
	assertExecutionWithParent(
		t,
		paymenttransaction.TypeVoid,
		[]string{
			paymenttransaction.TypeAuthorize,
		},
	)
}

func TestExecuteStripeRefundResolvesSaleOrCaptureParent(
	t *testing.T,
) {
	assertExecutionWithParent(
		t,
		paymenttransaction.TypeRefund,
		[]string{
			paymenttransaction.TypeSale,
			paymenttransaction.TypeCapture,
		},
	)
}

func TestExecuteReturnsParentNotFound(
	t *testing.T,
) {
	transaction :=
		testExecutionTransaction(
			paymenttransaction.TypeRefund,
		)

	service :=
		NewService(
			Dependencies{
				Transactions: &fakePaymentTransactionRepository{
					getByID: func(
						context.Context,
						string,
					) (*models.PaymentTransaction, error) {
						return transaction, nil
					},

					getLatestSuccessfulByPaymentAndTypes: func(
						context.Context,
						string,
						string,
						[]string,
					) (*models.PaymentTransaction, error) {
						return nil,
							repository.ErrNotFound
					},
				},

				PaymentTransactions: &fakePaymentTransactionService{},

				Stripe: &fakeStripeExecutor{},
			},
		)

	_, err :=
		service.Execute(
			context.Background(),
			transaction.ID,
		)

	if !errors.Is(
		err,
		ErrParentProviderTransactionNotFound,
	) {
		t.Fatalf(
			"expected ErrParentProviderTransactionNotFound, got %v",
			err,
		)
	}
}

func TestExecuteRejectsUnsupportedProvider(
	t *testing.T,
) {
	transaction :=
		testExecutionTransaction(
			paymenttransaction.TypeSale,
		)

	transaction.Provider =
		"OTHER_PROVIDER"

	service :=
		NewService(
			Dependencies{
				Transactions: &fakePaymentTransactionRepository{
					getByID: func(
						context.Context,
						string,
					) (*models.PaymentTransaction, error) {
						return transaction, nil
					},
				},

				PaymentTransactions: &fakePaymentTransactionService{},

				Stripe: &fakeStripeExecutor{},
			},
		)

	_, err :=
		service.Execute(
			context.Background(),
			transaction.ID,
		)

	if !errors.Is(
		err,
		ErrUnsupportedProvider,
	) {
		t.Fatalf(
			"expected ErrUnsupportedProvider, got %v",
			err,
		)
	}
}

func assertExecutionWithoutParent(
	t *testing.T,
	transactionType string,
) {
	t.Helper()

	transaction :=
		testExecutionTransaction(
			transactionType,
		)

	service :=
		NewService(
			Dependencies{
				Transactions: &fakePaymentTransactionRepository{
					getByID: func(
						context.Context,
						string,
					) (*models.PaymentTransaction, error) {
						return transaction, nil
					},

					getLatestSuccessfulByPaymentAndTypes: func(
						context.Context,
						string,
						string,
						[]string,
					) (*models.PaymentTransaction, error) {
						t.Fatal(
							"operation must not resolve parent",
						)

						return nil, nil
					},
				},

				PaymentTransactions: successfulFakePaymentTransactionService(
					transaction,
				),

				Stripe: &fakeStripeExecutor{
					execute: func(
						_ context.Context,
						req stripepayment.ExecuteRequest,
					) (*stripepayment.ExecuteResult, error) {
						if req.ParentProviderTransactionID != "" {
							t.Fatalf(
								"unexpected parent identity %q",
								req.ParentProviderTransactionID,
							)
						}

						return &stripepayment.ExecuteResult{
							ProviderTransactionID: "pi_new_123",

							Status: paymenttransaction.StatusProcessing,
						}, nil
					},
				},
			},
		)

	if _, err :=
		service.Execute(
			context.Background(),
			transaction.ID,
		); err != nil {

		t.Fatalf(
			"execute %s: %v",
			transactionType,
			err,
		)
	}
}

func assertExecutionWithParent(
	t *testing.T,
	transactionType string,
	wantParentTypes []string,
) {
	t.Helper()

	transaction :=
		testExecutionTransaction(
			transactionType,
		)

	parentProviderID :=
		"pi_parent_123"

	repo :=
		&fakePaymentTransactionRepository{
			getByID: func(
				context.Context,
				string,
			) (*models.PaymentTransaction, error) {
				return transaction, nil
			},

			getLatestSuccessfulByPaymentAndTypes: func(
				_ context.Context,
				paymentID string,
				provider string,
				transactionTypes []string,
			) (*models.PaymentTransaction, error) {
				if paymentID != transaction.PaymentID {
					t.Fatalf(
						"unexpected payment ID %q",
						paymentID,
					)
				}

				if provider != stripepayment.ProviderName {
					t.Fatalf(
						"unexpected provider %q",
						provider,
					)
				}

				if !equalStrings(
					transactionTypes,
					wantParentTypes,
				) {
					t.Fatalf(
						"parent types got %v want %v",
						transactionTypes,
						wantParentTypes,
					)
				}

				return &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: "parent-transaction-123",
					},

					Provider: stripepayment.ProviderName,

					ProviderTransactionID: &parentProviderID,

					Status: paymenttransaction.StatusSuccess,
				}, nil
			},
		}

	stripeExecutor :=
		&fakeStripeExecutor{
			execute: func(
				_ context.Context,
				req stripepayment.ExecuteRequest,
			) (*stripepayment.ExecuteResult, error) {
				if req.ParentProviderTransactionID !=
					parentProviderID {

					t.Fatalf(
						"parent identity got %q want %q",
						req.ParentProviderTransactionID,
						parentProviderID,
					)
				}

				return &stripepayment.ExecuteResult{
					ProviderTransactionID: "pi_result_123",

					Status: paymenttransaction.StatusProcessing,
				}, nil
			},
		}

	service :=
		NewService(
			Dependencies{
				Transactions: repo,

				PaymentTransactions: successfulFakePaymentTransactionService(
					transaction,
				),

				Stripe: stripeExecutor,
			},
		)

	if _, err :=
		service.Execute(
			context.Background(),
			transaction.ID,
		); err != nil {

		t.Fatalf(
			"execute %s: %v",
			transactionType,
			err,
		)
	}
}

func successfulFakePaymentTransactionService(
	transaction *models.PaymentTransaction,
) *fakePaymentTransactionService {
	return &fakePaymentTransactionService{
		applyResult: func(
			_ context.Context,
			transactionID string,
			req paymenttransaction.ApplyResultRequest,
		) (*models.PaymentTransaction, error) {
			return &models.PaymentTransaction{
				BaseModel: models.BaseModel{
					ID: transactionID,
				},

				Status: req.Status,

				ProviderTransactionID: req.ProviderTransactionID,
			}, nil
		},
	}
}

func testExecutionTransaction(
	transactionType string,
) *models.PaymentTransaction {
	idempotencyKey :=
		"execution-idempotency-key"

	return &models.PaymentTransaction{
		BaseModel: models.BaseModel{
			ID: "transaction-123",
		},

		PaymentID: "payment-123",

		Provider: stripepayment.ProviderName,

		IdempotencyKey: &idempotencyKey,

		TransactionType: transactionType,

		Status: paymenttransaction.StatusPending,

		Amount: "23.45",

		Currency: "EUR",
	}
}

func equalStrings(
	got []string,
	want []string,
) bool {
	if len(got) != len(want) {
		return false
	}

	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}

	return true
}

var _ repository.PaymentTransactionRepository = (*fakePaymentTransactionRepository)(nil)

var _ paymenttransaction.Service = (*fakePaymentTransactionService)(nil)

var _ stripepayment.Executor = (*fakeStripeExecutor)(nil)
