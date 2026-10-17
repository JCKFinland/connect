package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

const flexWebhookTestRoute = "/api/v1/flex/webhooks/stripe"

func TestFlexWebhookRouteDisabledWithoutHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	v1 := router.Group("/api/v1")

	registerFlexAuthorizationWebhookRoutes(v1, nil)

	request := httptest.NewRequest(
		http.MethodPost,
		flexWebhookTestRoute,
		strings.NewReader(`{}`),
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404 when FLEX handler is absent, got %d",
			recorder.Code,
		)
	}
}

func TestFlexWebhookRouteRegisteredWithoutJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)

	verificationCalls := 0
	reconciliationCalls := 0

	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{
			verify: func(
				_ context.Context,
				headers http.Header,
				body []byte,
			) (*stripe.VerifiedFlexAuthorizationEvent, error) {
				verificationCalls++

				if headers.Get("Authorization") != "" {
					t.Fatal("test must not supply a JWT")
				}

				if headers.Get("Stripe-Signature") == "" {
					t.Fatal("Stripe signature header missing")
				}

				return &stripe.VerifiedFlexAuthorizationEvent{
					EventID: "evt_public_route",
				}, nil
			},
		},
		&fakeFlexHTTPReconciler{
			reconcile: func(
				context.Context,
				*stripe.VerifiedFlexAuthorizationEvent,
			) (*postgresrepo.FlexAuthorization, error) {
				reconciliationCalls++

				return &postgresrepo.FlexAuthorization{
					Status: "AUTHORIZED",
				}, nil
			},
		},
	)

	router := gin.New()
	v1 := router.Group("/api/v1")

	registerFlexAuthorizationWebhookRoutes(v1, handler)

	request := httptest.NewRequest(
		http.MethodPost,
		flexWebhookTestRoute,
		strings.NewReader(`{"id":"evt_public_route"}`),
	)

	request.Header.Set(
		"Stripe-Signature",
		"test-signature",
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected public webhook route to return 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if verificationCalls != 1 ||
		reconciliationCalls != 1 {
		t.Fatalf(
			"unexpected calls: verify=%d reconcile=%d",
			verificationCalls,
			reconciliationCalls,
		)
	}
}

func TestFlexWebhookRouteDoesNotReplacePaymentCallbacks(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	v1 := router.Group("/api/v1")

	// Register the existing payment callback path as a
	// sentinel to verify FLEX registration leaves it intact.
	v1.POST(
		"/payment-callbacks/:provider",
		func(c *gin.Context) {
			c.Status(http.StatusAccepted)
		},
	)

	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{},
		&fakeFlexHTTPReconciler{},
	)

	registerFlexAuthorizationWebhookRoutes(v1, handler)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/stripe",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf(
			"existing payment callback route changed: got %d",
			recorder.Code,
		)
	}

	foundFlex := false
	foundPaymentCallback := false

	for _, route := range router.Routes() {
		if route.Method != http.MethodPost {
			continue
		}

		switch route.Path {
		case flexWebhookTestRoute:
			foundFlex = true

		case "/api/v1/payment-callbacks/:provider":
			foundPaymentCallback = true
		}
	}

	if !foundFlex || !foundPaymentCallback {
		t.Fatalf(
			"route isolation failed: flex=%v payment_callback=%v",
			foundFlex,
			foundPaymentCallback,
		)
	}
}
