package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/internal/services/paymentexecution"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type fakePaymentExecutionTransactionReader struct {
	getByID func(
		ctx context.Context,
		id string,
	) (*models.PaymentTransaction, error)
}

func (f *fakePaymentExecutionTransactionReader) GetByID(
	ctx context.Context,
	id string,
) (*models.PaymentTransaction, error) {
	if f.getByID == nil {
		panic(
			"unexpected payment transaction GetByID call",
		)
	}

	return f.getByID(
		ctx,
		id,
	)
}

type fakePaymentExecutionService struct {
	execute func(
		ctx context.Context,
		transactionID string,
	) (*paymentexecution.Result, error)
}

func (f *fakePaymentExecutionService) Execute(
	ctx context.Context,
	transactionID string,
) (*paymentexecution.Result, error) {
	if f.execute == nil {
		panic(
			"unexpected payment execution Execute call",
		)
	}

	return f.execute(
		ctx,
		transactionID,
	)
}

func TestPaymentExecutionRejectsMissingAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-123",
		)

	handler :=
		NewPaymentExecutionHandler(
			nil,
			nil,
			nil,
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusUnauthorized {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}
}

func TestPaymentExecutionAccessDeniedStopsBeforeTransactionLookup(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-123",
		)

	setPaymentTestUser(c)

	transactionLookupCalled := false
	executionCalled := false

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					context.Context,
					string,
					string,
				) (*models.Payment, error) {
					return nil,
						payment.ErrPaymentAccessDenied
				},
			},
			&fakePaymentExecutionTransactionReader{
				getByID: func(
					context.Context,
					string,
				) (*models.PaymentTransaction, error) {
					transactionLookupCalled = true

					return nil, nil
				},
			},
			&fakePaymentExecutionService{
				execute: func(
					context.Context,
					string,
				) (*paymentexecution.Result, error) {
					executionCalled = true

					return nil, nil
				},
			},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusForbidden {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusForbidden,
			recorder.Code,
		)
	}

	if transactionLookupCalled {
		t.Fatal(
			"transaction lookup must not run after payment authorization fails",
		)
	}

	if executionCalled {
		t.Fatal(
			"payment execution must not run after payment authorization fails",
		)
	}
}

func TestPaymentExecutionRejectsTransactionFromDifferentPayment(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-456",
		)

	user :=
		setPaymentTestUser(c)

	executionCalled := false

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					_ context.Context,
					id string,
					userID string,
				) (*models.Payment, error) {
					if id != "payment-123" {
						t.Fatalf(
							"unexpected payment ID %q",
							id,
						)
					}

					if userID != user.ID {
						t.Fatalf(
							"unexpected user ID %q",
							userID,
						)
					}

					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: "payment-123",
						},
					}, nil
				},
			},
			&fakePaymentExecutionTransactionReader{
				getByID: func(
					_ context.Context,
					id string,
				) (*models.PaymentTransaction, error) {
					if id != "transaction-456" {
						t.Fatalf(
							"unexpected transaction ID %q",
							id,
						)
					}

					return &models.PaymentTransaction{
						BaseModel: models.BaseModel{
							ID: "transaction-456",
						},

						PaymentID: "payment-other",
					}, nil
				},
			},
			&fakePaymentExecutionService{
				execute: func(
					context.Context,
					string,
				) (*paymentexecution.Result, error) {
					executionCalled = true

					return nil, nil
				},
			},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusNotFound {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}

	if executionCalled {
		t.Fatal(
			"payment execution must not run for a transaction belonging to another payment",
		)
	}
}

func TestPaymentExecutionTransactionNotFound(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"missing-transaction",
		)

	setPaymentTestUser(c)

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					context.Context,
					string,
					string,
				) (*models.Payment, error) {
					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: "payment-123",
						},
					}, nil
				},
			},
			&fakePaymentExecutionTransactionReader{
				getByID: func(
					context.Context,
					string,
				) (*models.PaymentTransaction, error) {
					return nil,
						repository.ErrNotFound
				},
			},
			&fakePaymentExecutionService{},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusNotFound {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}
}

func TestPaymentExecutionSuccessReturnsSafeCustomerActionResponse(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-123",
		)

	user :=
		setPaymentTestUser(c)

	providerTransactionID :=
		"pi_123"

	idempotencyKey :=
		"must-not-be-exposed"

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-123",
			},

			PaymentID: "payment-123",

			Provider: "STRIPE",

			ProviderTransactionID: &providerTransactionID,

			IdempotencyKey: &idempotencyKey,

			TransactionType: paymenttransaction.TypeSale,

			Status: paymenttransaction.StatusProcessing,

			Amount: "23.45",

			Currency: "EUR",
		}

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					_ context.Context,
					id string,
					userID string,
				) (*models.Payment, error) {
					if id != transaction.PaymentID {
						t.Fatalf(
							"unexpected payment ID %q",
							id,
						)
					}

					if userID != user.ID {
						t.Fatalf(
							"unexpected user ID %q",
							userID,
						)
					}

					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: transaction.PaymentID,
						},
					}, nil
				},
			},
			&fakePaymentExecutionTransactionReader{
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
			},
			&fakePaymentExecutionService{
				execute: func(
					_ context.Context,
					transactionID string,
				) (*paymentexecution.Result, error) {
					if transactionID !=
						transaction.ID {

						t.Fatalf(
							"unexpected executed transaction ID %q",
							transactionID,
						)
					}

					return &paymentexecution.Result{
						Transaction: transaction,

						ClientSecret: "pi_123_secret_test",

						RequiresCustomerAction: true,
					}, nil
				},
			},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusOK {

		t.Fatalf(
			"expected HTTP status %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body map[string]any

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	success, ok :=
		body["success"].(bool)

	if !ok || !success {
		t.Fatalf(
			"expected success=true, got %#v",
			body["success"],
		)
	}

	data, ok :=
		body["data"].(map[string]any)

	if !ok {
		t.Fatalf(
			"expected data object, got %#v",
			body["data"],
		)
	}

	if data["id"] !=
		transaction.ID {

		t.Fatalf(
			"transaction ID got %#v",
			data["id"],
		)
	}

	if data["payment_id"] !=
		transaction.PaymentID {

		t.Fatalf(
			"payment ID got %#v",
			data["payment_id"],
		)
	}

	if data["provider_transaction_id"] !=
		providerTransactionID {

		t.Fatalf(
			"provider transaction ID got %#v",
			data["provider_transaction_id"],
		)
	}

	if data["client_secret"] !=
		"pi_123_secret_test" {

		t.Fatalf(
			"client secret got %#v",
			data["client_secret"],
		)
	}

	requiresAction, ok :=
		data["requires_customer_action"].(bool)

	if !ok || !requiresAction {
		t.Fatalf(
			"expected requires_customer_action=true, got %#v",
			data["requires_customer_action"],
		)
	}

	for _, forbiddenField := range []string{
		"idempotency_key",
		"gateway_request",
		"gateway_response",
	} {
		if _, exists :=
			data[forbiddenField]; exists {

			t.Fatalf(
				"response must not expose %q",
				forbiddenField,
			)
		}
	}
}

func TestPaymentExecutionOmitsClientSecretWhenCustomerActionNotRequired(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-123",
		)

	setPaymentTestUser(c)

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-123",
			},

			PaymentID: "payment-123",

			Provider: "STRIPE",

			TransactionType: paymenttransaction.TypeAuthorize,

			Status: paymenttransaction.StatusSuccess,

			Amount: "23.45",

			Currency: "EUR",
		}

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					context.Context,
					string,
					string,
				) (*models.Payment, error) {
					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: "payment-123",
						},
					}, nil
				},
			},
			&fakePaymentExecutionTransactionReader{
				getByID: func(
					context.Context,
					string,
				) (*models.PaymentTransaction, error) {
					return transaction, nil
				},
			},
			&fakePaymentExecutionService{
				execute: func(
					context.Context,
					string,
				) (*paymentexecution.Result, error) {
					return &paymentexecution.Result{
						Transaction: transaction,

						ClientSecret: "must-not-be-returned",

						RequiresCustomerAction: false,
					}, nil
				},
			},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusOK {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}

	var body map[string]any

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	data, ok :=
		body["data"].(map[string]any)

	if !ok {
		t.Fatalf(
			"expected data object, got %#v",
			body["data"],
		)
	}

	if _, exists :=
		data["client_secret"]; exists {

		t.Fatal(
			"client_secret must be omitted when customer action is not required",
		)
	}
}

func TestPaymentExecutionParentNotFoundReturnsBadRequest(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-refund",
		)

	setPaymentTestUser(c)

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-refund",
			},

			PaymentID: "payment-123",
		}

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					context.Context,
					string,
					string,
				) (*models.Payment, error) {
					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: "payment-123",
						},
					}, nil
				},
			},
			&fakePaymentExecutionTransactionReader{
				getByID: func(
					context.Context,
					string,
				) (*models.PaymentTransaction, error) {
					return transaction, nil
				},
			},
			&fakePaymentExecutionService{
				execute: func(
					context.Context,
					string,
				) (*paymentexecution.Result, error) {
					return nil,
						paymentexecution.ErrParentProviderTransactionNotFound
				},
			},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusBadRequest {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusBadRequest,
			recorder.Code,
		)
	}
}

func TestPaymentExecutionInternalErrorDoesNotLeakProviderDetails(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder, c :=
		newPaymentExecutionTestContext(
			t,
			"payment-123",
			"transaction-123",
		)

	setPaymentTestUser(c)

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-123",
			},

			PaymentID: "payment-123",
		}

	const sensitiveProviderError = "Stripe secret upstream failure sk_test_should_not_leak"

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					context.Context,
					string,
					string,
				) (*models.Payment, error) {
					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: "payment-123",
						},
					}, nil
				},
			},
			&fakePaymentExecutionTransactionReader{
				getByID: func(
					context.Context,
					string,
				) (*models.PaymentTransaction, error) {
					return transaction, nil
				},
			},
			&fakePaymentExecutionService{
				execute: func(
					context.Context,
					string,
				) (*paymentexecution.Result, error) {
					return nil,
						errors.New(
							sensitiveProviderError,
						)
				},
			},
		)

	handler.Execute(c)

	if recorder.Code !=
		http.StatusInternalServerError {

		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusInternalServerError,
			recorder.Code,
		)
	}

	if body := recorder.Body.String(); containsString(
		body,
		sensitiveProviderError,
	) {

		t.Fatal(
			"internal provider error leaked into HTTP response",
		)
	}
}

func newPaymentExecutionTestContext(
	t *testing.T,
	paymentID string,
	transactionID string,
) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()

	recorder :=
		httptest.NewRecorder()

	c, _ :=
		gin.CreateTestContext(
			recorder,
		)

	c.Request =
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/payments/"+
				paymentID+
				"/transactions/"+
				transactionID+
				"/execute",
			nil,
		)

	c.Params =
		gin.Params{
			{
				Key: "id",

				Value: paymentID,
			},
			{
				Key: "transaction_id",

				Value: transactionID,
			},
		}

	return recorder, c
}

func containsString(
	value string,
	substring string,
) bool {
	if substring == "" {
		return true
	}

	if len(substring) >
		len(value) {

		return false
	}

	for index := 0; index <=
		len(value)-len(substring); index++ {

		if value[index:index+len(substring)] ==
			substring {

			return true
		}
	}

	return false
}

var _ paymentTransactionReader = (*fakePaymentExecutionTransactionReader)(nil)

var _ paymentexecution.Service = (*fakePaymentExecutionService)(nil)
