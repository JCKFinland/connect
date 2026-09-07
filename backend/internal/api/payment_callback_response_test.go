package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func TestPaymentCallbackResponseDoesNotLeakGatewaySecrets(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	rawPayload := []byte(
		`{
			"id":"evt_test_safe_response",
			"type":"payment_intent.succeeded",
			"data":{
				"object":{
					"id":"pi_test_safe_response",
					"client_secret":"pi_test_safe_response_secret_must_not_leak"
				}
			}
		}`,
	)

	registry :=
		paymentcallback.NewVerifierRegistry()

	err := registry.Register(
		"STRIPE",
		&fakeHTTPCallbackVerifier{
			verify: func(
				_ context.Context,
				_ http.Header,
				body []byte,
			) (*paymentcallback.VerifiedCallback, error) {
				if string(body) != string(rawPayload) {
					t.Fatalf(
						"unexpected callback body: %s",
						string(body),
					)
				}

				return &paymentcallback.VerifiedCallback{
					Provider: "STRIPE",

					ProviderTransactionID: "pi_test_safe_response",

					ProviderStatus: "SUCCESS",

					RawPayload: body,
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"register callback verifier: %v",
			err,
		)
	}

	idempotencyKey :=
		"must-not-leak-idempotency-key"

	providerTransactionID :=
		"pi_test_safe_response"

	processedAt :=
		time.Date(
			2026,
			time.September,
			7,
			20,
			32,
			13,
			0,
			time.UTC,
		)

	handler :=
		NewPaymentCallbackHandler(
			&fakePaymentCallbackService{
				apply: func(
					_ context.Context,
					req paymentcallback.ApplyProviderCallbackRequest,
				) (*models.PaymentTransaction, error) {
					return &models.PaymentTransaction{
						BaseModel: models.BaseModel{
							ID: "transaction-safe-response",
						},

						PaymentID: "payment-safe-response",

						TransactionReference: "txn_safe_response",

						Provider: req.Provider,

						ProviderTransactionID: &providerTransactionID,

						IdempotencyKey: &idempotencyKey,

						TransactionType: paymenttransaction.TypeSale,

						Status: paymenttransaction.StatusSuccess,

						Amount: "6.28",

						Currency: "EUR",

						GatewayResponse: json.RawMessage(rawPayload),

						ProcessedAt: &processedAt,
					}, nil
				},
			},
			registry,
		)

	router := gin.New()

	router.POST(
		"/api/v1/payment-callbacks/:provider",
		handler.Handle,
	)

	recorder :=
		httptest.NewRecorder()

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/payment-callbacks/STRIPE",
			strings.NewReader(
				string(rawPayload),
			),
		)

	router.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	body :=
		recorder.Body.String()

	for _, forbidden := range []string{
		"gateway_response",
		"gateway_request",
		"idempotency_key",
		"client_secret",
		"must-not-leak-idempotency-key",
		"pi_test_safe_response_secret_must_not_leak",
		"txn_safe_response",
		"evt_test_safe_response",
	} {
		if strings.Contains(
			body,
			forbidden,
		) {
			t.Fatalf(
				"callback response leaked forbidden value %q: %s",
				forbidden,
				body,
			)
		}
	}

	for _, expected := range []string{
		`"id":"transaction-safe-response"`,
		`"payment_id":"payment-safe-response"`,
		`"provider":"STRIPE"`,
		`"provider_transaction_id":"pi_test_safe_response"`,
		`"transaction_type":"SALE"`,
		`"status":"SUCCESS"`,
		`"amount":"6.28"`,
		`"currency":"EUR"`,
		`"processed_at":`,
	} {
		if !strings.Contains(
			body,
			expected,
		) {
			t.Fatalf(
				"callback response missing %q: %s",
				expected,
				body,
			)
		}
	}
}
