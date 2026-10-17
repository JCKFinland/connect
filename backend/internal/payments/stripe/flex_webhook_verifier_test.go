package stripe

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stripe/stripe-go/v86/webhook"
)

func signedFlexWebhookPayload(
	t *testing.T,
	eventType string,
	status string,
	modify func(map[string]interface{}),
) ([]byte, string) {
	t.Helper()

	intent := map[string]interface{}{
		"id":             "pi_flex_test",
		"object":         "payment_intent",
		"status":         status,
		"amount":         1800,
		"currency":       "eur",
		"capture_method": "manual",
		"metadata": map[string]interface{}{
			"connect_operation":        "FLEX_AUTHORIZE",
			"connect_authorization_id": "authorization_test",
			"connect_auction_id":       "auction_test",
			"connect_offer_id":         "offer_test",
			"connect_passenger_id":     "passenger_test",
		},
	}

	if modify != nil {
		modify(intent)
	}

	event := map[string]interface{}{
		"id":          "evt_flex_test",
		"object":      "event",
		"api_version": "2026-08-26.dahlia",
		"type":        eventType,
		"data": map[string]interface{}{
			"object": intent,
		},
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal Stripe event: %v", err)
	}

	signed := webhook.GenerateTestSignedPayload(
		&webhook.UnsignedPayload{
			Payload: payload,
			Secret:  testWebhookSecret,
		},
	)

	return signed.Payload, signed.Header
}

func verifySignedFlexWebhook(
	t *testing.T,
	verifier *FlexWebhookVerifier,
	payload []byte,
	signature string,
) (*VerifiedFlexAuthorizationEvent, error) {
	t.Helper()

	headers := make(http.Header)
	headers.Set("Stripe-Signature", signature)

	return verifier.Verify(
		context.Background(),
		headers,
		payload,
	)
}

func TestFlexWebhookVerifierAcceptsValidEvents(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		status    string
	}{
		{
			name:      "authorization capturable",
			eventType: "payment_intent.amount_capturable_updated",
			status:    "requires_capture",
		},
		{
			name:      "processing",
			eventType: "payment_intent.processing",
			status:    "processing",
		},
		{
			name:      "payment failed",
			eventType: "payment_intent.payment_failed",
			status:    "requires_payment_method",
		},
		{
			name:      "canceled",
			eventType: "payment_intent.canceled",
			status:    "canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier, err := NewFlexWebhookVerifier(
				testWebhookSecret,
			)
			if err != nil {
				t.Fatal(err)
			}

			payload, signature := signedFlexWebhookPayload(
				t,
				tt.eventType,
				tt.status,
				nil,
			)

			result, err := verifySignedFlexWebhook(
				t,
				verifier,
				payload,
				signature,
			)
			if err != nil {
				t.Fatalf("verify FLEX event: %v", err)
			}

			if result.EventID != "evt_flex_test" ||
				result.EventType != tt.eventType ||
				result.ProviderAuthorizationID != "pi_flex_test" ||
				string(result.ProviderStatus) != tt.status {
				t.Fatalf("unexpected provider identity: %+v", result)
			}

			if result.AuthorizationID != "authorization_test" ||
				result.AuctionID != "auction_test" ||
				result.OfferID != "offer_test" ||
				result.PassengerID != "passenger_test" {
				t.Fatalf("unexpected CONNECT identity: %+v", result)
			}

			if result.AmountCents != 1800 ||
				result.Currency != "EUR" ||
				string(result.CaptureMethod) != "manual" {
				t.Fatalf("unexpected financial identity: %+v", result)
			}
		})
	}
}

func TestFlexWebhookVerifierRejectsInvalidEvents(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		status    string
		modify    func(map[string]interface{})
	}{
		{
			name:      "unsupported succeeded event",
			eventType: "payment_intent.succeeded",
			status:    "succeeded",
		},
		{
			name:      "capturable event with processing status",
			eventType: "payment_intent.amount_capturable_updated",
			status:    "processing",
		},
		{
			name:      "processing event with capturable status",
			eventType: "payment_intent.processing",
			status:    "requires_capture",
		},
		{
			name:      "canceled event with processing status",
			eventType: "payment_intent.canceled",
			status:    "processing",
		},
		{
			name:      "failed event with capturable status",
			eventType: "payment_intent.payment_failed",
			status:    "requires_capture",
		},
		{
			name:      "missing provider identity",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				intent["id"] = ""
			},
		},
		{
			name:      "incorrect operation marker",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				metadata := intent["metadata"].(map[string]interface{})
				metadata["connect_operation"] = "SALE"
			},
		},
		{
			name:      "missing authorization identity",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				metadata := intent["metadata"].(map[string]interface{})
				delete(metadata, "connect_authorization_id")
			},
		},
		{
			name:      "missing auction identity",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				metadata := intent["metadata"].(map[string]interface{})
				delete(metadata, "connect_auction_id")
			},
		},
		{
			name:      "missing offer identity",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				metadata := intent["metadata"].(map[string]interface{})
				delete(metadata, "connect_offer_id")
			},
		},
		{
			name:      "missing passenger identity",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				metadata := intent["metadata"].(map[string]interface{})
				delete(metadata, "connect_passenger_id")
			},
		},
		{
			name:      "zero amount",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				intent["amount"] = 0
			},
		},
		{
			name:      "incorrect currency",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				intent["currency"] = "usd"
			},
		},
		{
			name:      "automatic capture",
			eventType: "payment_intent.processing",
			status:    "processing",
			modify: func(intent map[string]interface{}) {
				intent["capture_method"] = "automatic"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier, err := NewFlexWebhookVerifier(
				testWebhookSecret,
			)
			if err != nil {
				t.Fatal(err)
			}

			payload, signature := signedFlexWebhookPayload(
				t,
				tt.eventType,
				tt.status,
				tt.modify,
			)

			result, err := verifySignedFlexWebhook(
				t,
				verifier,
				payload,
				signature,
			)

			if err == nil {
				t.Fatalf(
					"expected rejection, got result: %+v",
					result,
				)
			}
		})
	}
}

func TestFlexWebhookVerifierRejectsTamperedSignature(t *testing.T) {
	verifier, err := NewFlexWebhookVerifier(testWebhookSecret)
	if err != nil {
		t.Fatal(err)
	}

	payload, signature := signedFlexWebhookPayload(
		t,
		"payment_intent.amount_capturable_updated",
		"requires_capture",
		nil,
	)

	payload = append([]byte(nil), payload...)
	payload[len(payload)-1] ^= 1

	result, err := verifySignedFlexWebhook(
		t,
		verifier,
		payload,
		signature,
	)

	if err == nil {
		t.Fatalf("tampered payload accepted: %+v", result)
	}
}

func TestFlexWebhookVerifierRejectsMissingSignature(t *testing.T) {
	verifier, err := NewFlexWebhookVerifier(testWebhookSecret)
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := signedFlexWebhookPayload(
		t,
		"payment_intent.processing",
		"processing",
		nil,
	)

	result, err := verifySignedFlexWebhook(
		t,
		verifier,
		payload,
		"",
	)

	if err == nil {
		t.Fatalf("unsigned payload accepted: %+v", result)
	}
}

func TestFlexWebhookVerifierRequiresSigningSecret(t *testing.T) {
	if _, err := NewFlexWebhookVerifier(""); err == nil {
		t.Fatal("expected missing signing secret rejection")
	}
}
