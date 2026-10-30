package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

func TestPaymentTransactionAuthorizationStopsInitiation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const internalDetail = "database authorization failure: private-hostname"

	tests := []struct {
		name        string
		serviceErr  error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "access denied",
			serviceErr:  payment.ErrPaymentAccessDenied,
			wantStatus:  http.StatusForbidden,
			wantMessage: "You are not authorized to create a transaction for this payment",
		},
		{
			name:        "payment not found",
			serviceErr:  pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
			wantMessage: "Payment not found",
		},
		{
			name:        "unexpected authorization failure",
			serviceErr:  errors.New(internalDetail),
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/v1/payments/payment-foreign/transactions",
				strings.NewReader(`{
                    "provider":"STRIPE",
                    "idempotency_key":"idem-authz-test",
                    "transaction_type":"SALE"
                }`),
			)

			c.Request.Header.Set(
				"Content-Type",
				"application/json",
			)

			c.Params = gin.Params{
				{
					Key:   "id",
					Value: "payment-foreign",
				},
			}

			user := setPaymentTestUser(c)

			authorizationCalled := false
			initiationCalled := false

			handler := NewPaymentTransactionHandler(
				&fakePaymentService{
					getByIDAuthorized: func(
						_ context.Context,
						paymentID string,
						userID string,
					) (*models.Payment, error) {
						authorizationCalled = true

						if paymentID != "payment-foreign" {
							t.Errorf(
								"payment ID: got %q",
								paymentID,
							)
						}

						if userID != user.ID {
							t.Errorf(
								"authenticated user ID: got %q, want %q",
								userID,
								user.ID,
							)
						}

						return nil, tt.serviceErr
					},
				},
				&fakePaymentTransactionService{
					initiate: func(
						context.Context,
						paymenttransaction.InitiateOperationRequest,
					) (*models.PaymentTransaction, error) {
						initiationCalled = true
						return nil, nil
					},
				},
			)

			handler.Initiate(c)

			if !authorizationCalled {
				t.Fatal("payment authorization was not performed")
			}

			if initiationCalled {
				t.Fatal(
					"transaction initiation executed despite failed payment authorization",
				)
			}

			if recorder.Code != tt.wantStatus {
				t.Fatalf(
					"HTTP status: got %d, want %d; body=%q",
					recorder.Code,
					tt.wantStatus,
					recorder.Body.String(),
				)
			}

			var body response.ErrorResponse

			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&body,
			); err != nil {
				t.Fatalf("decode error response: %v", err)
			}

			if body.Success {
				t.Fatal("expected success=false")
			}

			if body.Message != tt.wantMessage {
				t.Fatalf(
					"error message: got %q, want %q",
					body.Message,
					tt.wantMessage,
				)
			}

			if strings.Contains(
				recorder.Body.String(),
				internalDetail,
			) {
				t.Fatalf(
					"internal authorization error disclosed: %q",
					recorder.Body.String(),
				)
			}
		})
	}
}
