package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
)

func TestPaymentCallbackRouteNotRegisteredWithoutHandler(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	v1 := router.Group("/api/v1")

	registerPaymentCallbackRoutes(
		v1,
		nil,
	)

	recorder := httptest.NewRecorder()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/STRIPE",
		strings.NewReader(`{}`),
	)

	router.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected unconfigured callback route to return %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}
}

func TestPaymentCallbackRouteDoesNotRequireJWT(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	registry :=
		paymentcallback.NewVerifierRegistry()

	err := registry.Register(
		"STRIPE",
		&fakeHTTPCallbackVerifier{
			verify: func(
				_ context.Context,
				_ http.Header,
				rawBody []byte,
			) (*paymentcallback.VerifiedCallback, error) {
				return &paymentcallback.VerifiedCallback{
					Provider: "STRIPE",

					ProviderTransactionID: "pi_test_route",

					ProviderStatus: "SUCCESS",

					RawPayload: rawBody,
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

	handler :=
		NewPaymentCallbackHandler(
			&fakePaymentCallbackService{
				apply: func(
					_ context.Context,
					req paymentcallback.ApplyProviderCallbackRequest,
				) (*models.PaymentTransaction, error) {
					return &models.PaymentTransaction{
						BaseModel: models.BaseModel{
							ID: "transaction-route-test",
						},

						Provider: req.Provider,

						ProviderTransactionID: &req.ProviderTransactionID,

						Status: req.ProviderStatus,
					}, nil
				},
			},
			registry,
		)

	router := gin.New()

	v1 := router.Group("/api/v1")

	registerPaymentCallbackRoutes(
		v1,
		handler,
	)

	recorder := httptest.NewRecorder()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/STRIPE",
		strings.NewReader(
			`{"event":"test"}`,
		),
	)

	// Intentionally NO Authorization header.
	router.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected callback route without JWT to return %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}
}
