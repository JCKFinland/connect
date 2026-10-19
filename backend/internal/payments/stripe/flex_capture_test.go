package stripe

import (
	"context"
	"errors"
	"strings"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"
)

func validFlexCaptureRequest() FlexCaptureRequest {
	return FlexCaptureRequest{
		ProviderAuthorizationID: "pi_flex_capture_123",
		AmountCents:             2345,
		Currency:                "EUR",
		CaptureIdempotencyKey:   "flex-capture:settlement-123",
	}
}

func validFlexCapturedIntent() *stripego.PaymentIntent {
	return &stripego.PaymentIntent{
		ID:               "pi_flex_capture_123",
		Amount:           2345,
		AmountReceived:   2345,
		AmountCapturable: 0,
		Currency:         stripego.CurrencyEUR,
		CaptureMethod:    stripego.PaymentIntentCaptureMethodManual,
		Status:           stripego.PaymentIntentStatusSucceeded,
	}
}

func TestFlexCaptureAdapterSuccessfulCapture(t *testing.T) {
	req := validFlexCaptureRequest()
	calls := 0

	client := &recordingPaymentIntentClient{
		capture: func(
			_ context.Context,
			id string,
			params *stripego.PaymentIntentCaptureParams,
		) (*stripego.PaymentIntent, error) {
			calls++

			if id != req.ProviderAuthorizationID {
				t.Fatalf("provider ID = %q", id)
			}

			if params == nil ||
				params.AmountToCapture == nil ||
				*params.AmountToCapture != req.AmountCents {
				t.Fatal("capture amount mismatch")
			}

			if params.IdempotencyKey == nil ||
				*params.IdempotencyKey != req.CaptureIdempotencyKey {
				t.Fatal("capture idempotency key mismatch")
			}

			return validFlexCapturedIntent(), nil
		},
	}

	adapter := newFlexCaptureAdapterWithClient(client)

	result, err := adapter.ExecuteFlexCapture(
		context.Background(),
		req,
	)
	if err != nil {
		t.Fatalf("execute FLEX capture: %v", err)
	}

	if calls != 1 {
		t.Fatalf("capture calls = %d, want 1", calls)
	}

	if result == nil ||
		result.ProviderCaptureID != req.ProviderAuthorizationID ||
		result.ProviderStatus != stripego.PaymentIntentStatusSucceeded {
		t.Fatalf("unexpected capture result: %+v", result)
	}
}

func TestFlexCaptureAdapterProcessing(t *testing.T) {
	intent := validFlexCapturedIntent()
	intent.Status = stripego.PaymentIntentStatusProcessing
	intent.AmountReceived = 0

	client := &recordingPaymentIntentClient{
		capture: func(
			_ context.Context,
			_ string,
			_ *stripego.PaymentIntentCaptureParams,
		) (*stripego.PaymentIntent, error) {
			return intent, nil
		},
	}

	adapter := newFlexCaptureAdapterWithClient(client)

	result, err := adapter.ExecuteFlexCapture(
		context.Background(),
		validFlexCaptureRequest(),
	)
	if err != nil {
		t.Fatalf("processing capture: %v", err)
	}

	if result == nil ||
		result.ProviderStatus != stripego.PaymentIntentStatusProcessing {
		t.Fatalf("unexpected processing result: %+v", result)
	}
}

func TestFlexCaptureAdapterRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name   string
		change func(*FlexCaptureRequest)
	}{
		{
			name: "missing provider identity",
			change: func(r *FlexCaptureRequest) {
				r.ProviderAuthorizationID = ""
			},
		},
		{
			name: "missing idempotency key",
			change: func(r *FlexCaptureRequest) {
				r.CaptureIdempotencyKey = ""
			},
		},
		{
			name: "zero amount",
			change: func(r *FlexCaptureRequest) {
				r.AmountCents = 0
			},
		},
		{
			name: "negative amount",
			change: func(r *FlexCaptureRequest) {
				r.AmountCents = -1
			},
		},
		{
			name: "unsupported currency",
			change: func(r *FlexCaptureRequest) {
				r.Currency = "USD"
			},
		},
		{
			name: "provider identity whitespace",
			change: func(r *FlexCaptureRequest) {
				r.ProviderAuthorizationID = " pi_flex_capture_123 "
			},
		},
		{
			name: "idempotency key whitespace",
			change: func(r *FlexCaptureRequest) {
				r.CaptureIdempotencyKey = " flex-capture:settlement-123 "
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validFlexCaptureRequest()
			tc.change(&req)

			calls := 0

			client := &recordingPaymentIntentClient{
				capture: func(
					_ context.Context,
					_ string,
					_ *stripego.PaymentIntentCaptureParams,
				) (*stripego.PaymentIntent, error) {
					calls++
					return validFlexCapturedIntent(), nil
				},
			}

			adapter := newFlexCaptureAdapterWithClient(client)

			_, err := adapter.ExecuteFlexCapture(
				context.Background(),
				req,
			)

			if !errors.Is(err, ErrInvalidOperation) {
				t.Fatalf("expected invalid operation, got %v", err)
			}

			if calls != 0 {
				t.Fatalf("unexpected Stripe calls: %d", calls)
			}
		})
	}
}

func TestFlexCaptureAdapterRejectsInvalidProviderResponses(t *testing.T) {
	cases := []struct {
		name   string
		change func(*stripego.PaymentIntent)
	}{
		{
			name: "wrong provider identity",
			change: func(i *stripego.PaymentIntent) {
				i.ID = "pi_different"
			},
		},
		{
			name: "wrong amount",
			change: func(i *stripego.PaymentIntent) {
				i.Amount = 9999
			},
		},
		{
			name: "wrong currency",
			change: func(i *stripego.PaymentIntent) {
				i.Currency = stripego.CurrencyUSD
			},
		},
		{
			name: "not manual capture",
			change: func(i *stripego.PaymentIntent) {
				i.CaptureMethod =
					stripego.PaymentIntentCaptureMethodAutomatic
			},
		},
		{
			name: "partial capture",
			change: func(i *stripego.PaymentIntent) {
				i.AmountReceived = 1000
			},
		},
		{
			name: "remaining capturable amount",
			change: func(i *stripego.PaymentIntent) {
				i.AmountCapturable = 100
			},
		},
		{
			name: "unexpected provider status",
			change: func(i *stripego.PaymentIntent) {
				i.Status = stripego.PaymentIntentStatusCanceled
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingPaymentIntentClient{
				capture: func(
					_ context.Context,
					_ string,
					_ *stripego.PaymentIntentCaptureParams,
				) (*stripego.PaymentIntent, error) {
					intent := validFlexCapturedIntent()
					tc.change(intent)
					return intent, nil
				},
			}

			adapter := newFlexCaptureAdapterWithClient(client)

			_, err := adapter.ExecuteFlexCapture(
				context.Background(),
				validFlexCaptureRequest(),
			)

			if err == nil {
				t.Fatal("expected provider response rejection")
			}
		})
	}
}

func TestFlexCaptureAdapterProviderError(t *testing.T) {
	providerErr := errors.New("simulated Stripe timeout")

	client := &recordingPaymentIntentClient{
		capture: func(
			_ context.Context,
			_ string,
			_ *stripego.PaymentIntentCaptureParams,
		) (*stripego.PaymentIntent, error) {
			return nil, providerErr
		},
	}

	adapter := newFlexCaptureAdapterWithClient(client)

	_, err := adapter.ExecuteFlexCapture(
		context.Background(),
		validFlexCaptureRequest(),
	)

	if !errors.Is(err, providerErr) {
		t.Fatalf("provider error not preserved: %v", err)
	}
}

func TestFlexCaptureAdapterRejectsNilProviderResponse(t *testing.T) {
	client := &recordingPaymentIntentClient{
		capture: func(
			_ context.Context,
			_ string,
			_ *stripego.PaymentIntentCaptureParams,
		) (*stripego.PaymentIntent, error) {
			return nil, nil
		},
	}

	adapter := newFlexCaptureAdapterWithClient(client)

	_, err := adapter.ExecuteFlexCapture(
		context.Background(),
		validFlexCaptureRequest(),
	)

	if err == nil ||
		!strings.Contains(err.Error(), "no PaymentIntent") {
		t.Fatalf("expected missing provider response error, got %v", err)
	}
}
