package stripelinking

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v86"
)

func TestStripeProvisioningAdapterIdempotencyAndIdentity(
	t *testing.T,
) {
	actor := uuid.NewString()
	driver := uuid.NewString()
	company := uuid.NewString()
	key := "connect-stripe-provisioning:" + uuid.NewString()
	calls := 0

	adapter := NewStripeProvisioningAdapter(func(
		ctx context.Context,
		params *stripe.AccountParams,
	) (*stripe.Account, error) {
		calls++
		if ctx.Err() != nil || params == nil ||
			params.IdempotencyKey == nil ||
			*params.IdempotencyKey != key {
			t.Fatal("durable Stripe idempotency header missing")
		}

		if params.Type == nil || *params.Type != "express" ||
			params.Country == nil || *params.Country != "FI" ||
			params.DefaultCurrency == nil ||
			*params.DefaultCurrency != "eur" {
			t.Fatal("unexpected Stripe account configuration")
		}

		expected := map[string]string{
			"connect_actor_user_id":  actor,
			"connect_driver_user_id": driver,
			"connect_company_id":     company,
		}
		if len(params.Metadata) != len(expected) {
			t.Fatal("unexpected account metadata")
		}
		for field, value := range expected {
			if params.Metadata[field] != value {
				t.Fatalf("metadata mismatch: %s", field)
			}
		}

		for _, value := range params.Metadata {
			if value == key {
				t.Fatal("idempotency key leaked into metadata")
			}
		}

		return &stripe.Account{
			ID:              "acct_1234567890ABCDEF",
			Country:         "FI",
			DefaultCurrency: stripe.CurrencyEUR,
			Type:            stripe.AccountTypeExpress,
		}, nil
	})

	account, err := adapter.CreateProvisioningAccount(
		context.Background(), key, actor, driver, company,
	)
	if err != nil || account != "acct_1234567890ABCDEF" ||
		calls != 1 {
		t.Fatalf("unexpected result: %q %v calls=%d",
			account, err, calls)
	}
}

func TestStripeProvisioningAdapterRejectsInvalidInputs(
	t *testing.T,
) {
	actor := uuid.NewString()
	driver := uuid.NewString()
	company := uuid.NewString()
	key := "connect-stripe-provisioning:" + uuid.NewString()
	calls := 0

	adapter := NewStripeProvisioningAdapter(func(
		context.Context, *stripe.AccountParams,
	) (*stripe.Account, error) {
		calls++
		return nil, errors.New("unexpected provider invocation")
	})

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name    string
		ctx     context.Context
		key     string
		actor   string
		driver  string
		company string
	}{
		{"nil context", nil, key, actor, driver, company},
		{"canceled context", canceled, key, actor, driver, company},
		{"empty key", context.Background(), "", actor, driver, company},
		{"client key", context.Background(), "client-key", actor, driver, company},
		{"bad key UUID", context.Background(),
			"connect-stripe-provisioning:not-a-uuid",
			actor, driver, company},
		{"invalid actor", context.Background(), key, "invalid", driver, company},
		{"invalid driver", context.Background(), key, actor, "invalid", company},
		{"invalid company", context.Background(), key, actor, driver, "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account, err := adapter.CreateProvisioningAccount(
				tc.ctx, tc.key, tc.actor, tc.driver, tc.company,
			)
			if account != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf("unexpected result: %q %v", account, err)
			}
		})
	}

	if calls != 0 {
		t.Fatalf("provider invoked for invalid input: %d", calls)
	}

	var nilAdapter *StripeProvisioningAdapter
	account, err := nilAdapter.CreateProvisioningAccount(
		context.Background(), key, actor, driver, company,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatalf("nil adapter accepted: %q %v", account, err)
	}

	account, err = NewStripeProvisioningAdapter(nil).
		CreateProvisioningAccount(
			context.Background(), key, actor, driver, company,
		)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatalf("nil provider accepted: %q %v", account, err)
	}
}

func TestStripeProvisioningAdapterRejectsUntrustedResponse(
	t *testing.T,
) {
	actor := uuid.NewString()
	driver := uuid.NewString()
	company := uuid.NewString()
	key := "connect-stripe-provisioning:" + uuid.NewString()

	adapter := NewStripeProvisioningAdapter(func(
		context.Context, *stripe.AccountParams,
	) (*stripe.Account, error) {
		return &stripe.Account{
			ID:              "acct_1234567890ABCDEF",
			Country:         "US",
			DefaultCurrency: stripe.CurrencyUSD,
			Type:            stripe.AccountTypeExpress,
		}, nil
	})

	account, err := adapter.CreateProvisioningAccount(
		context.Background(), key, actor, driver, company,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatalf("untrusted response accepted: %q %v", account, err)
	}
}

func TestStripeProvisioningAdapterProviderFailurePaths(t *testing.T) {
	actor := uuid.NewString()
	driver := uuid.NewString()
	company := uuid.NewString()
	key := "connect-stripe-provisioning:" + uuid.NewString()

	validAccount := func() *stripe.Account {
		return &stripe.Account{
			ID:              "acct_1234567890ABCDEF",
			Country:         "FI",
			DefaultCurrency: stripe.CurrencyEUR,
			Type:            stripe.AccountTypeExpress,
		}
	}

	cases := []struct {
		name   string
		create func(context.Context, *stripe.AccountParams) (
			*stripe.Account, error,
		)
	}{
		{
			name: "provider error",
			create: func(
				context.Context, *stripe.AccountParams,
			) (*stripe.Account, error) {
				return nil, errors.New("synthetic provider failure")
			},
		},
		{
			name: "nil provider response",
			create: func(
				context.Context, *stripe.AccountParams,
			) (*stripe.Account, error) {
				return nil, nil
			},
		},
		{
			name: "malformed account ID",
			create: func(
				context.Context, *stripe.AccountParams,
			) (*stripe.Account, error) {
				account := validAccount()
				account.ID = "invalid-account"
				return account, nil
			},
		},
		{
			name: "deleted account",
			create: func(
				context.Context, *stripe.AccountParams,
			) (*stripe.Account, error) {
				account := validAccount()
				account.Deleted = true
				return account, nil
			},
		},
		{
			name: "wrong account type",
			create: func(
				context.Context, *stripe.AccountParams,
			) (*stripe.Account, error) {
				account := validAccount()
				account.Type = stripe.AccountTypeStandard
				return account, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			calls := 0
			create := tc.create

			adapter := NewStripeProvisioningAdapter(func(
				requestCtx context.Context,
				params *stripe.AccountParams,
			) (*stripe.Account, error) {
				calls++

				if params == nil ||
					params.IdempotencyKey == nil ||
					*params.IdempotencyKey != key {
					t.Fatal("durable idempotency key missing")
				}

				return create(requestCtx, params)
			})

			accountID, err := adapter.CreateProvisioningAccount(
				ctx, key, actor, driver, company,
			)

			if accountID != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf(
					"unsafe provider result accepted: account=%q err=%v",
					accountID, err,
				)
			}

			if calls != 1 {
				t.Fatalf(
					"unexpected provider invocation count: %d",
					calls,
				)
			}
		})
	}
}

// The adapter must pass through validated provider evidence even
// when the request is canceled during the provider invocation.
func TestStripeProvisioningAdapterPreservesCanceledProviderEvidence(
	t *testing.T,
) {
	actor := uuid.NewString()
	driver := uuid.NewString()
	company := uuid.NewString()
	key := "connect-stripe-provisioning:" + uuid.NewString()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	adapter := NewStripeProvisioningAdapter(func(
		requestCtx context.Context,
		params *stripe.AccountParams,
	) (*stripe.Account, error) {
		calls++
		if requestCtx.Err() != nil || params == nil ||
			params.IdempotencyKey == nil ||
			*params.IdempotencyKey != key {
			t.Fatal("invalid provider invocation or idempotency key")
		}

		cancel()
		return &stripe.Account{
			ID:              "acct_1234567890ABCDEF",
			Country:         "FI",
			DefaultCurrency: stripe.CurrencyEUR,
			Type:            stripe.AccountTypeExpress,
		}, nil
	})

	accountID, err := adapter.CreateProvisioningAccount(
		ctx, key, actor, driver, company,
	)
	if err != nil ||
		accountID != "acct_1234567890ABCDEF" ||
		calls != 1 {
		t.Fatalf(
			"trusted adapter response lost: %q %v calls=%d",
			accountID, err, calls,
		)
	}
}
