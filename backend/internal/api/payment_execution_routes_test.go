package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymentexecution"
)

func TestPaymentExecutionRouteNotRegisteredWithoutHandler(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	v1 := router.Group("/api/v1")

	authMiddleware :=
		middleware.NewAuthMiddleware(
			nil,
			nil,
		)

	registerPaymentExecutionRoutes(
		v1,
		authMiddleware,
		nil,
	)

	recorder := httptest.NewRecorder()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payments/payment-123/transactions/transaction-123/execute",
		nil,
	)

	router.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected unconfigured execution route to return %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}
}

func TestPaymentExecutionRouteRequiresJWT(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	executionCalled := false

	handler :=
		NewPaymentExecutionHandler(
			&fakePaymentService{},
			&fakePaymentExecutionTransactionReader{},
			&fakePaymentExecutionService{
				execute: func(
					context.Context,
					string,
				) (*paymentexecution.Result, error) {
					executionCalled = true

					return &paymentexecution.Result{
						Transaction: &models.PaymentTransaction{},
					}, nil
				},
			},
		)

	router := gin.New()

	v1 := router.Group("/api/v1")

	authMiddleware :=
		middleware.NewAuthMiddleware(
			nil,
			nil,
		)

	registerPaymentExecutionRoutes(
		v1,
		authMiddleware,
		handler,
	)

	recorder := httptest.NewRecorder()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payments/payment-123/transactions/transaction-123/execute",
		nil,
	)

	// Intentionally NO Authorization header.
	router.ServeHTTP(
		recorder,
		request,
	)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected protected execution route without JWT to return %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}

	if executionCalled {
		t.Fatal(
			"payment execution handler must not run without authentication",
		)
	}
}
