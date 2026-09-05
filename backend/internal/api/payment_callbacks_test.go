package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
)

type fakePaymentCallbackService struct {
	apply func(
		ctx context.Context,
		req paymentcallback.ApplyProviderCallbackRequest,
	) (*models.PaymentTransaction, error)
}

func (f *fakePaymentCallbackService) ApplyProviderCallback(
	ctx context.Context,
	req paymentcallback.ApplyProviderCallbackRequest,
) (*models.PaymentTransaction, error) {
	if f.apply == nil {
		panic("unexpected ApplyProviderCallback call")
	}

	return f.apply(ctx, req)
}

type fakeHTTPCallbackVerifier struct {
	verify func(
		ctx context.Context,
		headers http.Header,
		rawBody []byte,
	) (*paymentcallback.VerifiedCallback, error)
}

func (f *fakeHTTPCallbackVerifier) Verify(
	ctx context.Context,
	headers http.Header,
	rawBody []byte,
) (*paymentcallback.VerifiedCallback, error) {
	if f.verify == nil {
		panic("unexpected Verify call")
	}

	return f.verify(
		ctx,
		headers,
		rawBody,
	)
}

func TestPaymentCallbackRejectsUnknownProvider(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/UNKNOWN",
		strings.NewReader(`{}`),
	)

	c.Params = gin.Params{
		{
			Key:   "provider",
			Value: "UNKNOWN",
		},
	}

	handler := NewPaymentCallbackHandler(
		&fakePaymentCallbackService{},
		paymentcallback.NewVerifierRegistry(),
	)

	handler.Handle(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}
}

func TestPaymentCallbackRejectsVerificationFailure(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	registry :=
		paymentcallback.NewVerifierRegistry()

	err := registry.Register(
		"TEST_PROVIDER",
		&fakeHTTPCallbackVerifier{
			verify: func(
				context.Context,
				http.Header,
				[]byte,
			) (*paymentcallback.VerifiedCallback, error) {
				return nil,
					paymentcallback.ErrCallbackVerificationFailed
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"register verifier: %v",
			err,
		)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/TEST_PROVIDER",
		strings.NewReader(`{"event":"test"}`),
	)

	c.Params = gin.Params{
		{
			Key:   "provider",
			Value: "TEST_PROVIDER",
		},
	}

	handler := NewPaymentCallbackHandler(
		&fakePaymentCallbackService{},
		registry,
	)

	handler.Handle(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}
}

func TestPaymentCallbackVerifiedRequestIsForwarded(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	const rawPayload = `{"event":"payment.completed"}`

	registry :=
		paymentcallback.NewVerifierRegistry()

	err := registry.Register(
		"TEST_PROVIDER",
		&fakeHTTPCallbackVerifier{
			verify: func(
				_ context.Context,
				_ http.Header,
				rawBody []byte,
			) (*paymentcallback.VerifiedCallback, error) {
				if string(rawBody) != rawPayload {
					t.Fatalf(
						"raw callback payload changed: %q",
						string(rawBody),
					)
				}

				return &paymentcallback.VerifiedCallback{
					Provider: "TEST_PROVIDER",

					ProviderTransactionID: "provider-123",

					ProviderStatus: "SUCCESS",
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"register verifier: %v",
			err,
		)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/TEST_PROVIDER",
		strings.NewReader(rawPayload),
	)

	c.Params = gin.Params{
		{
			Key:   "provider",
			Value: "TEST_PROVIDER",
		},
	}

	handler := NewPaymentCallbackHandler(
		&fakePaymentCallbackService{
			apply: func(
				_ context.Context,
				req paymentcallback.ApplyProviderCallbackRequest,
			) (*models.PaymentTransaction, error) {
				if req.Provider != "TEST_PROVIDER" {
					t.Fatalf(
						"unexpected provider %q",
						req.Provider,
					)
				}

				if req.ProviderTransactionID !=
					"provider-123" {

					t.Fatalf(
						"unexpected provider transaction ID %q",
						req.ProviderTransactionID,
					)
				}

				if req.ProviderStatus != "SUCCESS" {
					t.Fatalf(
						"unexpected provider status %q",
						req.ProviderStatus,
					)
				}

				if string(req.RawPayload) != rawPayload {
					t.Fatalf(
						"raw payload mismatch: %q",
						string(req.RawPayload),
					)
				}

				return &models.PaymentTransaction{
					BaseModel: models.BaseModel{
						ID: "transaction-123",
					},

					Status: "SUCCESS",
				}, nil
			},
		},
		registry,
	)

	handler.Handle(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}
}

var _ paymentcallback.Service = (*fakePaymentCallbackService)(nil)

var _ paymentcallback.Verifier = (*fakeHTTPCallbackVerifier)(nil)

var _ = errors.Is

func TestPaymentCallbackRejectsVerifierProviderMismatch(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	registry :=
		paymentcallback.NewVerifierRegistry()

	err := registry.Register(
		"TEST_PROVIDER",
		&fakeHTTPCallbackVerifier{
			verify: func(
				context.Context,
				http.Header,
				[]byte,
			) (*paymentcallback.VerifiedCallback, error) {
				return &paymentcallback.VerifiedCallback{
					Provider: "OTHER_PROVIDER",

					ProviderTransactionID: "provider-123",

					ProviderStatus: "SUCCESS",
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"register verifier: %v",
			err,
		)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/payment-callbacks/TEST_PROVIDER",
		strings.NewReader(
			`{"event":"payment.completed"}`,
		),
	)

	c.Params = gin.Params{
		{
			Key:   "provider",
			Value: "TEST_PROVIDER",
		},
	}

	handler := NewPaymentCallbackHandler(
		&fakePaymentCallbackService{
			apply: func(
				context.Context,
				paymentcallback.ApplyProviderCallbackRequest,
			) (*models.PaymentTransaction, error) {
				t.Fatal(
					"callback service must not be called after provider mismatch",
				)

				return nil, nil
			},
		},
		registry,
	)

	handler.Handle(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}
}
