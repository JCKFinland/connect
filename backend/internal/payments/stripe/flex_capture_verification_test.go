package stripe

import (
	"context"
	"errors"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"
)

type flexCaptureVerificationClient struct {
	fakePaymentIntentClient

	intent *stripego.PaymentIntent
	err    error

	retrieveCalls int
	captureCalls  int
}

func (c *flexCaptureVerificationClient) Retrieve(
	_ context.Context,
	_ string,
	_ *stripego.PaymentIntentRetrieveParams,
) (*stripego.PaymentIntent, error) {
	c.retrieveCalls++
	return c.intent, c.err
}

func (c *flexCaptureVerificationClient) Capture(
	_ context.Context,
	_ string,
	_ *stripego.PaymentIntentCaptureParams,
) (*stripego.PaymentIntent, error) {
	c.captureCalls++
	return nil, errors.New("capture must never be called")
}

func validFlexCaptureVerificationRequest() FlexCaptureVerificationRequest {
	return FlexCaptureVerificationRequest{
		ProviderAuthorizationID: "pi_flex_test_123",
		AuthorizationID:         "auth-test-123",
		AuctionID:               "auction-test-123",
		OfferID:                 "offer-test-123",
		PassengerID:             "passenger-test-123",
		AmountCents:             2345,
		Currency:                "EUR",
	}
}

func validFlexCapturePaymentIntent() *stripego.PaymentIntent {
	return &stripego.PaymentIntent{
		ID:               "pi_flex_test_123",
		Amount:           2345,
		AmountReceived:   2345,
		AmountCapturable: 0,
		Currency:         stripego.CurrencyEUR,
		CaptureMethod:    stripego.PaymentIntentCaptureMethodManual,
		Status:           stripego.PaymentIntentStatusSucceeded,
		Metadata: map[string]string{
			"connect_operation":        "FLEX_AUTHORIZE",
			"connect_authorization_id": "auth-test-123",
			"connect_auction_id":       "auction-test-123",
			"connect_offer_id":         "offer-test-123",
			"connect_passenger_id":     "passenger-test-123",
		},
	}
}

func TestFlexCaptureVerifierAcceptsCapturedPaymentIntent(t *testing.T) {
	client := &flexCaptureVerificationClient{
		intent: validFlexCapturePaymentIntent(),
	}

	verifier := newExecutorWithPaymentIntents(client).(FlexCaptureVerifier)

	result, err := verifier.VerifyFlexCapture(
		context.Background(),
		validFlexCaptureVerificationRequest(),
	)
	if err != nil {
		t.Fatalf("verify captured PaymentIntent: %v", err)
	}

	if result == nil ||
		!result.Captured ||
		result.ProviderCaptureID != "pi_flex_test_123" ||
		result.ProviderStatus != stripego.PaymentIntentStatusSucceeded {
		t.Fatalf("unexpected capture verification result: %+v", result)
	}

	if client.retrieveCalls != 1 || client.captureCalls != 0 {
		t.Fatalf(
			"unexpected provider calls: retrieve=%d capture=%d",
			client.retrieveCalls,
			client.captureCalls,
		)
	}
}

func TestFlexCaptureVerifierAcceptsUncapturedAuthorization(
	t *testing.T,
) {
	intent := validFlexCapturePaymentIntent()
	intent.Status = stripego.PaymentIntentStatusRequiresCapture
	intent.AmountReceived = 0
	intent.AmountCapturable = 2345

	client := &flexCaptureVerificationClient{intent: intent}
	verifier := newExecutorWithPaymentIntents(client).(FlexCaptureVerifier)

	result, err := verifier.VerifyFlexCapture(
		context.Background(),
		validFlexCaptureVerificationRequest(),
	)
	if err != nil {
		t.Fatalf("verify uncaptured authorization: %v", err)
	}

	if result == nil || result.Captured {
		t.Fatalf("uncaptured authorization marked captured: %+v", result)
	}

	if client.captureCalls != 0 {
		t.Fatal("verification attempted to capture funds")
	}
}

func TestFlexCaptureVerifierRejectsInvalidProviderState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*stripego.PaymentIntent)
	}{
		{
			name: "wrong provider identity",
			mutate: func(p *stripego.PaymentIntent) {
				p.ID = "pi_wrong"
			},
		},
		{
			name: "wrong authorized amount",
			mutate: func(p *stripego.PaymentIntent) {
				p.Amount = 9999
			},
		},
		{
			name: "wrong captured amount",
			mutate: func(p *stripego.PaymentIntent) {
				p.AmountReceived = 100
			},
		},
		{
			name: "remaining capturable balance",
			mutate: func(p *stripego.PaymentIntent) {
				p.AmountCapturable = 100
			},
		},
		{
			name: "wrong currency",
			mutate: func(p *stripego.PaymentIntent) {
				p.Currency = stripego.CurrencyUSD
			},
		},
		{
			name: "automatic capture method",
			mutate: func(p *stripego.PaymentIntent) {
				p.CaptureMethod =
					stripego.PaymentIntentCaptureMethodAutomatic
			},
		},
		{
			name: "wrong operation metadata",
			mutate: func(p *stripego.PaymentIntent) {
				p.Metadata["connect_operation"] = "OTHER"
			},
		},
		{
			name: "wrong authorization metadata",
			mutate: func(p *stripego.PaymentIntent) {
				p.Metadata["connect_authorization_id"] = "wrong"
			},
		},
		{
			name: "wrong auction metadata",
			mutate: func(p *stripego.PaymentIntent) {
				p.Metadata["connect_auction_id"] = "wrong"
			},
		},
		{
			name: "wrong offer metadata",
			mutate: func(p *stripego.PaymentIntent) {
				p.Metadata["connect_offer_id"] = "wrong"
			},
		},
		{
			name: "wrong passenger metadata",
			mutate: func(p *stripego.PaymentIntent) {
				p.Metadata["connect_passenger_id"] = "wrong"
			},
		},
		{
			name: "unsupported provider status",
			mutate: func(p *stripego.PaymentIntent) {
				p.Status =
					stripego.PaymentIntentStatusRequiresPaymentMethod
			},
		},
		{
			name: "uncaptured amount mismatch",
			mutate: func(p *stripego.PaymentIntent) {
				p.Status = stripego.PaymentIntentStatusRequiresCapture
				p.AmountReceived = 0
				p.AmountCapturable = 100
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := validFlexCapturePaymentIntent()
			tt.mutate(intent)

			client := &flexCaptureVerificationClient{intent: intent}
			verifier := newExecutorWithPaymentIntents(client).(FlexCaptureVerifier)

			result, err := verifier.VerifyFlexCapture(
				context.Background(),
				validFlexCaptureVerificationRequest(),
			)

			if err == nil {
				t.Fatalf(
					"invalid provider state accepted: %+v",
					result,
				)
			}

			if client.captureCalls != 0 {
				t.Fatal("verification attempted to capture funds")
			}
		})
	}
}

func TestFlexCaptureVerifierRejectsProviderRetrievalFailure(
	t *testing.T,
) {
	client := &flexCaptureVerificationClient{
		err: errors.New("simulated Stripe network failure"),
	}

	verifier := newExecutorWithPaymentIntents(client).(FlexCaptureVerifier)

	result, err := verifier.VerifyFlexCapture(
		context.Background(),
		validFlexCaptureVerificationRequest(),
	)

	if err == nil || result != nil {
		t.Fatalf(
			"provider retrieval failure accepted: result=%+v err=%v",
			result,
			err,
		)
	}

	if client.captureCalls != 0 {
		t.Fatal("verification attempted to capture funds")
	}
}
