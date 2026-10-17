package stripe

import (
	"context"
	"errors"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"
)

func flexTestRequest() FlexAuthorizationRequest {
	return FlexAuthorizationRequest{
		AuthorizationID: "authorization-123",
		AuctionID:       "auction-123",
		OfferID:         "offer-123",
		PassengerID:     "passenger-123",
		AmountCents:     2345,
		Currency:        "EUR",
		IdempotencyKey:  "flex-authorize:auction-123",
	}
}

func flexTestIntent(
	req FlexAuthorizationRequest,
	status stripego.PaymentIntentStatus,
) *stripego.PaymentIntent {
	return &stripego.PaymentIntent{
		ID:            "pi_flex_123",
		Amount:        req.AmountCents,
		Currency:      stripego.CurrencyEUR,
		CaptureMethod: stripego.PaymentIntentCaptureMethodManual,
		Status:        status,
		ClientSecret:  "pi_flex_123_secret_test",
		Metadata: map[string]string{
			"connect_operation":        "FLEX_AUTHORIZE",
			"connect_authorization_id": req.AuthorizationID,
			"connect_auction_id":       req.AuctionID,
			"connect_offer_id":         req.OfferID,
			"connect_passenger_id":     req.PassengerID,
		},
	}
}

func TestFlexAdapterCreatesManualCaptureIntent(t *testing.T) {
	req := flexTestRequest()
	calls := 0

	client := &recordingPaymentIntentClient{
		create: func(
			_ context.Context,
			params *stripego.PaymentIntentCreateParams,
		) (*stripego.PaymentIntent, error) {
			calls++

			if params.Amount == nil ||
				*params.Amount != req.AmountCents {
				t.Fatal("incorrect Stripe amount")
			}
			if params.Currency == nil ||
				*params.Currency != "eur" {
				t.Fatal("incorrect Stripe currency")
			}
			if params.CaptureMethod == nil ||
				*params.CaptureMethod != "manual" {
				t.Fatal("manual capture not configured")
			}
			if params.IdempotencyKey == nil ||
				*params.IdempotencyKey != req.IdempotencyKey {
				t.Fatal("incorrect Stripe idempotency key")
			}
			if params.AutomaticPaymentMethods == nil ||
				params.AutomaticPaymentMethods.Enabled == nil ||
				!*params.AutomaticPaymentMethods.Enabled {
				t.Fatal("automatic payment methods not enabled")
			}

			expected := flexTestIntent(
				req,
				stripego.PaymentIntentStatusRequiresPaymentMethod,
			)
			for key, value := range expected.Metadata {
				if params.Metadata[key] != value {
					t.Fatalf("incorrect metadata: %s", key)
				}
			}

			return expected, nil
		},
	}

	adapter := newFlexAuthorizationAdapterWithClient(client)
	result, err := adapter.ExecuteFlexAuthorization(
		context.Background(), req,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected one create call, got %d", calls)
	}
	if result.ProviderAuthorizationID != "pi_flex_123" ||
		!result.RequiresCustomerAction ||
		result.Authorized {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFlexAdapterRecoversByRetrieval(t *testing.T) {
	req := flexTestRequest()
	req.ProviderAuthorizationID = "pi_flex_123"
	retrieveCalls := 0

	client := &recordingPaymentIntentClient{
		retrieve: func(
			_ context.Context,
			id string,
			_ *stripego.PaymentIntentRetrieveParams,
		) (*stripego.PaymentIntent, error) {
			retrieveCalls++
			if id != req.ProviderAuthorizationID {
				t.Fatalf("unexpected retrieval ID: %s", id)
			}
			return flexTestIntent(
				req,
				stripego.PaymentIntentStatusRequiresCapture,
			), nil
		},
	}

	result, err := newFlexAuthorizationAdapterWithClient(
		client,
	).ExecuteFlexAuthorization(context.Background(), req)

	if err != nil {
		t.Fatal(err)
	}
	if retrieveCalls != 1 || !result.Authorized ||
		result.RequiresCustomerAction {
		t.Fatalf("unexpected recovery result: %+v", result)
	}
}

func TestFlexAdapterRejectsProviderIdentityMismatch(t *testing.T) {
	req := flexTestRequest()
	req.ProviderAuthorizationID = "pi_expected"

	client := &recordingPaymentIntentClient{
		retrieve: func(
			_ context.Context,
			_ string,
			_ *stripego.PaymentIntentRetrieveParams,
		) (*stripego.PaymentIntent, error) {
			return flexTestIntent(
				req,
				stripego.PaymentIntentStatusRequiresCapture,
			), nil
		},
	}

	_, err := newFlexAuthorizationAdapterWithClient(
		client,
	).ExecuteFlexAuthorization(context.Background(), req)

	if !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("expected identity rejection, got %v", err)
	}
}

func TestFlexAdapterRejectsTamperedProviderResponse(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*stripego.PaymentIntent)
	}{
		{
			name: "amount",
			tamper: func(pi *stripego.PaymentIntent) {
				pi.Amount++
			},
		},
		{
			name: "currency",
			tamper: func(pi *stripego.PaymentIntent) {
				pi.Currency = stripego.CurrencyUSD
			},
		},
		{
			name: "capture method",
			tamper: func(pi *stripego.PaymentIntent) {
				pi.CaptureMethod =
					stripego.PaymentIntentCaptureMethodAutomatic
			},
		},
		{
			name: "authorization metadata",
			tamper: func(pi *stripego.PaymentIntent) {
				pi.Metadata["connect_authorization_id"] = "wrong"
			},
		},
		{
			name: "passenger metadata",
			tamper: func(pi *stripego.PaymentIntent) {
				delete(pi.Metadata, "connect_passenger_id")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := flexTestRequest()

			client := &recordingPaymentIntentClient{
				create: func(
					_ context.Context,
					_ *stripego.PaymentIntentCreateParams,
				) (*stripego.PaymentIntent, error) {
					intent := flexTestIntent(
						req,
						stripego.PaymentIntentStatusRequiresCapture,
					)
					tc.tamper(intent)
					return intent, nil
				},
			}

			_, err := newFlexAuthorizationAdapterWithClient(
				client,
			).ExecuteFlexAuthorization(context.Background(), req)

			if !errors.Is(err, ErrInvalidOperation) {
				t.Fatalf("expected provider rejection, got %v", err)
			}
		})
	}
}

func TestFlexAdapterRejectsInvalidRequestWithoutProviderCall(t *testing.T) {
	req := flexTestRequest()
	req.AmountCents = 0

	client := &recordingPaymentIntentClient{}

	_, err := newFlexAuthorizationAdapterWithClient(
		client,
	).ExecuteFlexAuthorization(context.Background(), req)

	if !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}

func TestFlexAdapterPropagatesProviderError(t *testing.T) {
	req := flexTestRequest()
	providerErr := errors.New("provider timeout")

	client := &recordingPaymentIntentClient{
		create: func(
			_ context.Context,
			_ *stripego.PaymentIntentCreateParams,
		) (*stripego.PaymentIntent, error) {
			return nil, providerErr
		},
	}

	_, err := newFlexAuthorizationAdapterWithClient(
		client,
	).ExecuteFlexAuthorization(context.Background(), req)

	if !errors.Is(err, providerErr) {
		t.Fatalf("expected original provider error, got %v", err)
	}
}
