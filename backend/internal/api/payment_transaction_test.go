package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type fakePaymentTransactionService struct {
	initiate func(
		ctx context.Context,
		req paymenttransaction.InitiateOperationRequest,
	) (*models.PaymentTransaction, error)
}

func (f *fakePaymentTransactionService) InitiateOperation(
	ctx context.Context,
	req paymenttransaction.InitiateOperationRequest,
) (*models.PaymentTransaction, error) {
	if f.initiate == nil {
		panic("unexpected InitiateOperation call")
	}

	return f.initiate(
		ctx,
		req,
	)
}

func (f *fakePaymentTransactionService) ApplyResult(
	context.Context,
	string,
	paymenttransaction.ApplyResultRequest,
) (*models.PaymentTransaction, error) {
	panic("unexpected ApplyResult call")
}

func TestPaymentTransactionInitiateSuccess(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payments/payment-123/transactions",
		strings.NewReader(
			`{
				"provider":"stripe",
				"idempotency_key":"idem-sale-123",
				"transaction_type":"sale"
			}`,
		),
	)

	c.Request.Header.Set(
		"Content-Type",
		"application/json",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "payment-123",
		},
	}

	user := setPaymentTestUser(c)

	handler :=
		NewPaymentTransactionHandler(
			&fakePaymentService{
				getByIDAuthorized: func(
					_ context.Context,
					id string,
					userID string,
				) (*models.Payment, error) {
					if id != "payment-123" {
						t.Fatalf(
							"payment ID got %q",
							id,
						)
					}

					if userID != user.ID {
						t.Fatalf(
							"user ID got %q",
							userID,
						)
					}

					return &models.Payment{
						BaseModel: models.BaseModel{
							ID: "payment-123",
						},
						CustomerID: user.ID,
						Status:     "PENDING",
						Amount:     "6.28",
						Currency:   "EUR",
					}, nil
				},
			},
			&fakePaymentTransactionService{
				initiate: func(
					_ context.Context,
					req paymenttransaction.InitiateOperationRequest,
				) (*models.PaymentTransaction, error) {
					if req.PaymentID != "payment-123" {
						t.Fatalf(
							"payment ID got %q",
							req.PaymentID,
						)
					}

					if req.Provider != "STRIPE" {
						t.Fatalf(
							"provider got %q want STRIPE",
							req.Provider,
						)
					}

					if req.IdempotencyKey != "idem-sale-123" {
						t.Fatalf(
							"idempotency key got %q",
							req.IdempotencyKey,
						)
					}

					if req.TransactionType != paymenttransaction.TypeSale {
						t.Fatalf(
							"transaction type got %q want %q",
							req.TransactionType,
							paymenttransaction.TypeSale,
						)
					}

					return &models.PaymentTransaction{
						BaseModel: models.BaseModel{
							ID: "transaction-123",
						},
						PaymentID:       req.PaymentID,
						Provider:        req.Provider,
						TransactionType: req.TransactionType,
						Status:          paymenttransaction.StatusPending,
						Amount:          "6.28",
						Currency:        "EUR",
						IdempotencyKey:  &req.IdempotencyKey,
					}, nil
				},
			},
		)

	handler.Initiate(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusCreated,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Success bool `json:"success"`

		Message string `json:"message"`

		Data models.PaymentTransaction `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if !body.Success {
		t.Fatal("expected success=true")
	}

	if body.Data.ID != "transaction-123" {
		t.Fatalf(
			"transaction ID got %q",
			body.Data.ID,
		)
	}

	if body.Data.Provider != "STRIPE" {
		t.Fatalf(
			"provider got %q",
			body.Data.Provider,
		)
	}

	if body.Data.TransactionType !=
		paymenttransaction.TypeSale {

		t.Fatalf(
			"transaction type got %q",
			body.Data.TransactionType,
		)
	}
}

func TestPaymentTransactionInitiateRejectsMissingAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payments/payment-123/transactions",
		strings.NewReader(
			`{
				"provider":"STRIPE",
				"idempotency_key":"idem-sale-123",
				"transaction_type":"SALE"
			}`,
		),
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "payment-123",
		},
	}

	handler :=
		NewPaymentTransactionHandler(
			nil,
			nil,
		)

	handler.Initiate(c)

	if recorder.Code !=
		http.StatusUnauthorized {

		t.Fatalf(
			"expected HTTP %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}
}

var _ paymenttransaction.Service = (*fakePaymentTransactionService)(nil)
