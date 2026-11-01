package stripelinking

import (
	"context"
	"errors"
	"testing"

	stripe "github.com/stripe/stripe-go/v86"
)

type fakeStripeAccountCreateClient struct {
	calls   int
	ctx     context.Context
	params  *stripe.AccountCreateParams
	account *stripe.Account
	err     error
}

func (f *fakeStripeAccountCreateClient) Create(
	ctx context.Context,
	params *stripe.AccountCreateParams,
) (*stripe.Account, error) {
	f.calls++
	f.ctx = ctx
	f.params = params
	return f.account, f.err
}

func TestStripeAccountCreateTransportRequest(t *testing.T) {
	ctx := context.WithValue(
		context.Background(),
		struct{ name string }{"transport"},
		"trusted",
	)

	client := &fakeStripeAccountCreateClient{
		account: &stripe.Account{ID: "acct_1234567890abcdef"},
	}
	create := NewStripeAccountCreateTransport(client)

	original := &stripe.AccountParams{
		Type:            stripe.String("express"),
		Country:         stripe.String("FI"),
		DefaultCurrency: stripe.String("eur"),
		Metadata: map[string]string{
			"connect_actor_user_id":  "actor",
			"connect_driver_user_id": "driver",
			"connect_company_id":     "company",
		},
	}
	original.SetIdempotencyKey("connect-stripe-provisioning:durable")

	account, err := create(ctx, original)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if account != client.account || client.calls != 1 {
		t.Fatal("transport did not preserve provider response")
	}
	if client.ctx != ctx {
		t.Fatal("request context was not forwarded")
	}

	p := client.params
	if p == nil ||
		p.Type == nil || *p.Type != "express" ||
		p.Country == nil || *p.Country != "FI" ||
		p.DefaultCurrency == nil || *p.DefaultCurrency != "eur" {
		t.Fatal("FI/EUR Express parameters changed")
	}
	if p.IdempotencyKey == nil ||
		*p.IdempotencyKey != "connect-stripe-provisioning:durable" {
		t.Fatal("durable idempotency key not forwarded")
	}
	for key, value := range original.Metadata {
		if p.Metadata[key] != value {
			t.Fatalf("metadata mismatch for %s", key)
		}
	}
	p.Metadata["connect_company_id"] = "changed"
	if original.Metadata["connect_company_id"] != "company" {
		t.Fatal("transport aliased original metadata")
	}
}

func TestStripeAccountCreateTransportRejectsUnsafeInput(t *testing.T) {
	client := &fakeStripeAccountCreateClient{}
	create := NewStripeAccountCreateTransport(client)

	valid := &stripe.AccountParams{}
	valid.SetIdempotencyKey("connect-stripe-provisioning:durable")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name   string
		ctx    context.Context
		params *stripe.AccountParams
	}{
		{"nil context", nil, valid},
		{"nil params", context.Background(), nil},
		{"missing key", context.Background(), &stripe.AccountParams{}},
		{"canceled context", canceled, valid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := create(tc.ctx, tc.params)
			if err == nil {
				t.Fatal("unsafe request accepted")
			}
		})
	}

	if client.calls != 0 {
		t.Fatal("unsafe request reached provider client")
	}

	missing := NewStripeAccountCreateTransport(nil)
	if _, err := missing(context.Background(), valid); err == nil {
		t.Fatal("missing provider client accepted")
	}
}

func TestStripeAccountCreateTransportPreservesProviderEvidence(t *testing.T) {
	providerErr := errors.New("provider returned an ambiguous error")
	account := &stripe.Account{ID: "acct_1234567890abcdef"}
	client := &fakeStripeAccountCreateClient{
		account: account,
		err:     providerErr,
	}
	create := NewStripeAccountCreateTransport(client)

	params := &stripe.AccountParams{}
	params.SetIdempotencyKey("connect-stripe-provisioning:durable")

	returned, err := create(context.Background(), params)
	if returned != account || !errors.Is(err, providerErr) {
		t.Fatal("provider response or error was discarded")
	}
	if client.calls != 1 {
		t.Fatal("unexpected provider call count")
	}
}
