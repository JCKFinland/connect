package stripe

import (
	"context"
	"errors"
	"net/http"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

const testWebhookSecret = "whsec_test_connect"

func signedStripePayload(
	t *testing.T,
	eventType string,
	paymentIntentID string,
) ([]byte, string) {
	t.Helper()

	payload := []byte(
		`{
			"id":"evt_test_connect",
			"object":"event",
			"api_version":"` + stripego.APIVersion + `",
			"type":"` + eventType + `",
			"data":{
				"object":{
					"id":"` + paymentIntentID + `",
					"object":"payment_intent"
				}
			}
		}`,
	)

	signed :=
		webhook.GenerateTestSignedPayload(
			&webhook.UnsignedPayload{
				Payload: payload,
				Secret:  testWebhookSecret,
			},
		)

	return signed.Payload, signed.Header
}

func TestWebhookVerifierAcceptsSignedPaymentIntentEvents(
	t *testing.T,
) {
	tests := []struct {
		name       string
		eventType  string
		wantStatus string
	}{
		{
			name:       "processing",
			eventType:  "payment_intent.processing",
			wantStatus: paymenttransaction.StatusProcessing,
		},
		{
			name:       "succeeded",
			eventType:  "payment_intent.succeeded",
			wantStatus: paymenttransaction.StatusSuccess,
		},
		{
			name:       "failed",
			eventType:  "payment_intent.payment_failed",
			wantStatus: paymenttransaction.StatusFailed,
		},
		{
			name:       "cancelled",
			eventType:  "payment_intent.canceled",
			wantStatus: paymenttransaction.StatusCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				verifier, err :=
					NewWebhookVerifier(
						testWebhookSecret,
					)
				if err != nil {
					t.Fatalf(
						"create verifier: %v",
						err,
					)
				}

				rawBody, signature :=
					signedStripePayload(
						t,
						tt.eventType,
						"pi_test_connect",
					)

				headers := make(http.Header)

				headers.Set(
					"Stripe-Signature",
					signature,
				)

				result, err :=
					verifier.Verify(
						context.Background(),
						headers,
						rawBody,
					)
				if err != nil {
					t.Fatalf(
						"verify Stripe callback: %v",
						err,
					)
				}

				if result.Provider != ProviderName {
					t.Fatalf(
						"expected provider %s, got %s",
						ProviderName,
						result.Provider,
					)
				}

				if result.ProviderTransactionID !=
					"pi_test_connect" {

					t.Fatalf(
						"expected PaymentIntent ID pi_test_connect, got %s",
						result.ProviderTransactionID,
					)
				}

				if result.ProviderStatus !=
					tt.wantStatus {

					t.Fatalf(
						"expected status %s, got %s",
						tt.wantStatus,
						result.ProviderStatus,
					)
				}

				if string(result.RawPayload) !=
					string(rawBody) {

					t.Fatal(
						"verified callback raw payload changed",
					)
				}
			},
		)
	}
}

func TestWebhookVerifierRejectsInvalidSignature(
	t *testing.T,
) {
	verifier, err :=
		NewWebhookVerifier(
			testWebhookSecret,
		)
	if err != nil {
		t.Fatalf(
			"create verifier: %v",
			err,
		)
	}

	rawBody, _ :=
		signedStripePayload(
			t,
			"payment_intent.succeeded",
			"pi_test_connect",
		)

	headers := make(http.Header)

	headers.Set(
		"Stripe-Signature",
		"t=1,v1=deadbeef",
	)

	_, err =
		verifier.Verify(
			context.Background(),
			headers,
			rawBody,
		)

	if !errors.Is(
		err,
		paymentcallback.ErrCallbackVerificationFailed,
	) {
		t.Fatalf(
			"expected ErrCallbackVerificationFailed, got %v",
			err,
		)
	}
}

func TestWebhookVerifierRejectsMissingSignature(
	t *testing.T,
) {
	verifier, err :=
		NewWebhookVerifier(
			testWebhookSecret,
		)
	if err != nil {
		t.Fatalf(
			"create verifier: %v",
			err,
		)
	}

	rawBody, _ :=
		signedStripePayload(
			t,
			"payment_intent.succeeded",
			"pi_test_connect",
		)

	_, err =
		verifier.Verify(
			context.Background(),
			make(http.Header),
			rawBody,
		)

	if !errors.Is(
		err,
		paymentcallback.ErrCallbackVerificationFailed,
	) {
		t.Fatalf(
			"expected ErrCallbackVerificationFailed, got %v",
			err,
		)
	}
}

func TestWebhookVerifierRejectsUnsupportedEvent(
	t *testing.T,
) {
	verifier, err :=
		NewWebhookVerifier(
			testWebhookSecret,
		)
	if err != nil {
		t.Fatalf(
			"create verifier: %v",
			err,
		)
	}

	rawBody, signature :=
		signedStripePayload(
			t,
			"customer.created",
			"cus_test_connect",
		)

	headers := make(http.Header)

	headers.Set(
		"Stripe-Signature",
		signature,
	)

	_, err =
		verifier.Verify(
			context.Background(),
			headers,
			rawBody,
		)

	if !errors.Is(
		err,
		paymentcallback.ErrInvalidCallback,
	) {
		t.Fatalf(
			"expected ErrInvalidCallback, got %v",
			err,
		)
	}
}

func TestWebhookVerifierRequiresSigningSecret(
	t *testing.T,
) {
	_, err :=
		NewWebhookVerifier(
			"   ",
		)

	if err == nil {
		t.Fatal(
			"expected empty signing secret to fail",
		)
	}
}
