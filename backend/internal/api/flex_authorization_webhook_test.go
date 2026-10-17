package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
)

type fakeFlexHTTPVerifier struct {
	verify func(
		context.Context,
		http.Header,
		[]byte,
	) (*stripe.VerifiedFlexAuthorizationEvent, error)
}

func (f *fakeFlexHTTPVerifier) Verify(
	ctx context.Context,
	headers http.Header,
	body []byte,
) (*stripe.VerifiedFlexAuthorizationEvent, error) {
	if f.verify == nil {
		panic("unexpected FLEX verifier invocation")
	}
	return f.verify(ctx, headers, body)
}

type fakeFlexHTTPReconciler struct {
	reconcile func(
		context.Context,
		*stripe.VerifiedFlexAuthorizationEvent,
	) (*postgresrepo.FlexAuthorization, error)
}

func (f *fakeFlexHTTPReconciler) ReconcileWebhook(
	ctx context.Context,
	event *stripe.VerifiedFlexAuthorizationEvent,
) (*postgresrepo.FlexAuthorization, error) {
	if f.reconcile == nil {
		panic("unexpected FLEX reconciliation invocation")
	}
	return f.reconcile(ctx, event)
}

func serveFlexWebhookTestRequest(
	handler *FlexAuthorizationWebhookHandler,
	body string,
) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST(
		"/api/v1/flex/webhooks/stripe",
		handler.Handle,
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/flex/webhooks/stripe",
		strings.NewReader(body),
	)

	request.Header.Set(
		"Stripe-Signature",
		"test-signature",
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestFlexWebhookHTTPRejectsInvalidSignature(
	t *testing.T,
) {
	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{
			verify: func(
				context.Context,
				http.Header,
				[]byte,
			) (*stripe.VerifiedFlexAuthorizationEvent, error) {
				return nil, errors.New("invalid signature")
			},
		},
		&fakeFlexHTTPReconciler{},
	)

	recorder := serveFlexWebhookTestRequest(
		handler,
		`{"id":"evt_invalid"}`,
	)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected 401, got %d",
			recorder.Code,
		)
	}

	if strings.Contains(
		recorder.Body.String(),
		"invalid signature",
	) {
		t.Fatal("internal verifier error leaked")
	}
}

func TestFlexWebhookHTTPRejectsOversizedBody(
	t *testing.T,
) {
	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{},
		&fakeFlexHTTPReconciler{},
	)

	recorder := serveFlexWebhookTestRequest(
		handler,
		strings.Repeat(
			"x",
			maximumFlexWebhookBodyBytes+1,
		),
	)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf(
			"expected 413, got %d",
			recorder.Code,
		)
	}
}

func TestFlexWebhookHTTPProcessesVerifiedEvent(
	t *testing.T,
) {
	const rawPayload = `{"id":"evt_valid"}`

	verifiedEvent := &stripe.VerifiedFlexAuthorizationEvent{
		EventID: "evt_valid",
	}

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

				if string(body) != rawPayload {
					t.Fatal("raw webhook body changed")
				}

				if headers.Get("Stripe-Signature") == "" {
					t.Fatal("signature header missing")
				}

				return verifiedEvent, nil
			},
		},
		&fakeFlexHTTPReconciler{
			reconcile: func(
				_ context.Context,
				event *stripe.VerifiedFlexAuthorizationEvent,
			) (*postgresrepo.FlexAuthorization, error) {
				reconciliationCalls++

				if event != verifiedEvent {
					t.Fatal("verified event identity changed")
				}

				return &postgresrepo.FlexAuthorization{}, nil
			},
		},
	)

	recorder := serveFlexWebhookTestRequest(
		handler,
		rawPayload,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if verificationCalls != 1 ||
		reconciliationCalls != 1 {
		t.Fatalf(
			"unexpected call counts: verify=%d reconcile=%d",
			verificationCalls,
			reconciliationCalls,
		)
	}
}

func TestFlexWebhookHTTPReturnsRetryableFailure(
	t *testing.T,
) {
	const sensitiveError = "stripe secret sk_test_must_not_leak"

	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{
			verify: func(
				context.Context,
				http.Header,
				[]byte,
			) (*stripe.VerifiedFlexAuthorizationEvent, error) {
				return &stripe.VerifiedFlexAuthorizationEvent{
					EventID: "evt_retry",
				}, nil
			},
		},
		&fakeFlexHTTPReconciler{
			reconcile: func(
				context.Context,
				*stripe.VerifiedFlexAuthorizationEvent,
			) (*postgresrepo.FlexAuthorization, error) {
				return nil, errors.New(sensitiveError)
			},
		},
	)

	recorder := serveFlexWebhookTestRequest(
		handler,
		`{"id":"evt_retry"}`,
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected 503, got %d",
			recorder.Code,
		)
	}

	if strings.Contains(
		recorder.Body.String(),
		sensitiveError,
	) {
		t.Fatal("sensitive provider error leaked")
	}
}

func TestFlexWebhookHTTPServiceErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		serviceError   error
		expectedStatus int
	}{
		{
			name:           "invalid event",
			serviceError:   flexauthorization.ErrInvalidFlexWebhook,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "authorization not ready",
			serviceError:   flexauthorization.ErrFlexWebhookNotReady,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "event processing",
			serviceError:   flexauthorization.ErrFlexWebhookProcessing,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "service unavailable",
			serviceError:   flexauthorization.ErrServiceUnavailable,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "wrapped processing error",
			serviceError: errors.Join(
				errors.New("sensitive internal error"),
				flexauthorization.ErrFlexWebhookProcessing,
			),
			expectedStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewFlexAuthorizationWebhookHandler(
				&fakeFlexHTTPVerifier{
					verify: func(
						context.Context,
						http.Header,
						[]byte,
					) (*stripe.VerifiedFlexAuthorizationEvent, error) {
						return &stripe.VerifiedFlexAuthorizationEvent{
							EventID: "evt_error_mapping",
						}, nil
					},
				},
				&fakeFlexHTTPReconciler{
					reconcile: func(
						context.Context,
						*stripe.VerifiedFlexAuthorizationEvent,
					) (*postgresrepo.FlexAuthorization, error) {
						return nil, tt.serviceError
					},
				},
			)

			recorder := serveFlexWebhookTestRequest(
				handler,
				`{"id":"evt_error_mapping"}`,
			)

			if recorder.Code != tt.expectedStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tt.expectedStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if strings.Contains(
				recorder.Body.String(),
				"sensitive internal error",
			) {
				t.Fatal("internal service error leaked")
			}
		})
	}
}

func TestFlexWebhookHTTPAcknowledgesIdempotentReplay(
	t *testing.T,
) {
	verificationCalls := 0
	reconciliationCalls := 0

	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{
			verify: func(
				context.Context,
				http.Header,
				[]byte,
			) (*stripe.VerifiedFlexAuthorizationEvent, error) {
				verificationCalls++

				return &stripe.VerifiedFlexAuthorizationEvent{
					EventID: "evt_replay",
				}, nil
			},
		},
		&fakeFlexHTTPReconciler{
			reconcile: func(
				context.Context,
				*stripe.VerifiedFlexAuthorizationEvent,
			) (*postgresrepo.FlexAuthorization, error) {
				reconciliationCalls++

				// The real service handles authorized replay
				// without creating another PaymentIntent.
				return &postgresrepo.FlexAuthorization{
					Status: "AUTHORIZED",
				}, nil
			},
		},
	)

	for i := 0; i < 2; i++ {
		recorder := serveFlexWebhookTestRequest(
			handler,
			`{"id":"evt_replay"}`,
		)

		if recorder.Code != http.StatusOK {
			t.Fatalf(
				"delivery %d: expected 200, got %d",
				i+1,
				recorder.Code,
			)
		}
	}

	if verificationCalls != 2 ||
		reconciliationCalls != 2 {
		t.Fatalf(
			"unexpected delivery counts: verify=%d reconcile=%d",
			verificationCalls,
			reconciliationCalls,
		)
	}
}

func TestFlexWebhookHTTPRejectsNilReconciliationResult(
	t *testing.T,
) {
	handler := NewFlexAuthorizationWebhookHandler(
		&fakeFlexHTTPVerifier{
			verify: func(
				context.Context,
				http.Header,
				[]byte,
			) (*stripe.VerifiedFlexAuthorizationEvent, error) {
				return &stripe.VerifiedFlexAuthorizationEvent{
					EventID: "evt_nil_result",
				}, nil
			},
		},
		&fakeFlexHTTPReconciler{
			reconcile: func(
				context.Context,
				*stripe.VerifiedFlexAuthorizationEvent,
			) (*postgresrepo.FlexAuthorization, error) {
				return nil, nil
			},
		},
	)

	recorder := serveFlexWebhookTestRequest(
		handler,
		`{"id":"evt_nil_result"}`,
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected HTTP 503, got %d",
			recorder.Code,
		)
	}
}
