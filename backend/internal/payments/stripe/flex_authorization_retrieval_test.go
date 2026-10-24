package stripe

import (
	"context"
	"errors"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"
)

type retrievalOnlyFake struct {
	calls  int
	intent *stripego.PaymentIntent
	err    error
}

func (f *retrievalOnlyFake) Retrieve(
	_ context.Context,
	_ string,
	_ *stripego.PaymentIntentRetrieveParams,
) (*stripego.PaymentIntent, error) {
	f.calls++
	return f.intent, f.err
}

func retrievalRequest() FlexAuthorizationRequest {
	return FlexAuthorizationRequest{
		AuthorizationID:         "auth-1",
		AuctionID:               "auction-1",
		OfferID:                 "offer-1",
		PassengerID:             "passenger-1",
		AmountCents:             1500,
		Currency:                "EUR",
		ProviderAuthorizationID: "pi_existing_1",
	}
}

func retrievalIntent() *stripego.PaymentIntent {
	return &stripego.PaymentIntent{
		ID:            "pi_existing_1",
		Amount:        1500,
		Currency:      stripego.CurrencyEUR,
		CaptureMethod: stripego.PaymentIntentCaptureMethodManual,
		Status:        stripego.PaymentIntentStatusRequiresCapture,
		Metadata: map[string]string{
			"connect_operation":        "FLEX_AUTHORIZE",
			"connect_authorization_id": "auth-1",
			"connect_auction_id":       "auction-1",
			"connect_offer_id":         "offer-1",
			"connect_passenger_id":     "passenger-1",
		},
	}
}

func TestRetrievalOnlyAdapterSuccessfulVerification(t *testing.T) {
	fake := &retrievalOnlyFake{
		intent: retrievalIntent(),
	}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	result, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		retrievalRequest(),
	)

	if err != nil {
		t.Fatalf("unexpected retrieval error: %v", err)
	}

	if result == nil || !result.Authorized {
		t.Fatal("expected verified authorization")
	}

	if fake.calls != 1 {
		t.Fatalf("expected one retrieval, got %d", fake.calls)
	}
}

func TestRetrievalOnlyAdapterRejectsMissingProviderID(t *testing.T) {
	fake := &retrievalOnlyFake{}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	request := retrievalRequest()
	request.ProviderAuthorizationID = ""

	_, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		request,
	)

	if err == nil {
		t.Fatal("expected missing provider ID rejection")
	}

	if fake.calls != 0 {
		t.Fatal("provider called without existing PaymentIntent ID")
	}
}

func TestRetrievalOnlyAdapterRejectsTamperedAmount(t *testing.T) {
	intent := retrievalIntent()
	intent.Amount = 9999

	fake := &retrievalOnlyFake{
		intent: intent,
	}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	_, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		retrievalRequest(),
	)

	if err == nil {
		t.Fatal("expected financial identity rejection")
	}
}

func TestRetrievalOnlyAdapterRejectsMetadataMismatch(t *testing.T) {
	intent := retrievalIntent()
	intent.Metadata["connect_passenger_id"] = "wrong-passenger"

	fake := &retrievalOnlyFake{
		intent: intent,
	}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	_, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		retrievalRequest(),
	)

	if err == nil {
		t.Fatal("expected metadata mismatch rejection")
	}
}

func TestRetrievalOnlyAdapterRejectsAutomaticCapture(t *testing.T) {
	intent := retrievalIntent()
	intent.CaptureMethod = stripego.PaymentIntentCaptureMethodAutomatic

	fake := &retrievalOnlyFake{
		intent: intent,
	}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	_, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		retrievalRequest(),
	)

	if err == nil {
		t.Fatal("expected automatic capture rejection")
	}
}

func TestRetrievalOnlyAdapterRejectsUnsupportedStatus(t *testing.T) {
	intent := retrievalIntent()
	intent.Status = stripego.PaymentIntentStatusSucceeded

	fake := &retrievalOnlyFake{
		intent: intent,
	}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	_, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		retrievalRequest(),
	)

	if err == nil {
		t.Fatal("expected unsupported provider status rejection")
	}
}

func TestRetrievalOnlyAdapterPropagatesProviderError(t *testing.T) {
	fake := &retrievalOnlyFake{
		err: errors.New("provider unavailable"),
	}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	_, err := adapter.RetrieveFlexAuthorization(
		context.Background(),
		retrievalRequest(),
	)

	if err == nil {
		t.Fatal("expected provider retrieval failure")
	}

	if fake.calls != 1 {
		t.Fatal("expected exactly one retrieval attempt")
	}
}

func TestRetrievalOnlyAdapterRejectsCancelledContext(t *testing.T) {
	fake := &retrievalOnlyFake{}

	adapter := &flexAuthorizationRetriever{
		paymentIntents: fake,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := adapter.RetrieveFlexAuthorization(
		ctx,
		retrievalRequest(),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	if fake.calls != 0 {
		t.Fatal("provider called after cancellation")
	}
}
