package stripelinking

import (
	"context"
	"errors"
	"testing"

	stripe "github.com/stripe/stripe-go/v86"
)

const (
	provisionActor   = "11111111-1111-4111-8111-111111111111"
	provisionDriver  = "33333333-3333-4333-8333-333333333333"
	provisionCompany = "44444444-4444-4444-8444-444444444444"
	provisionAccount = "acct_1234567890123456"
)

func validProvisionAccount() *stripe.Account {
	return &stripe.Account{
		ID:              provisionAccount,
		Country:         "FI",
		DefaultCurrency: stripe.CurrencyEUR,
		Type:            stripe.AccountTypeExpress,
	}
}

func TestStripeProvisioningControlledParameters(t *testing.T) {
	calls := 0
	provisioner := NewAccountProvisioner(func(
		_ context.Context,
		params *stripe.AccountParams,
	) (*stripe.Account, error) {
		calls++

		if params == nil ||
			params.Type == nil ||
			*params.Type != "express" ||
			params.Country == nil ||
			*params.Country != "FI" ||
			params.DefaultCurrency == nil ||
			*params.DefaultCurrency != "eur" {
			t.Fatalf("unexpected account parameters: %+v", params)
		}

		expected := map[string]string{
			"connect_actor_user_id":  provisionActor,
			"connect_driver_user_id": provisionDriver,
			"connect_company_id":     provisionCompany,
		}

		if len(params.Metadata) != len(expected) {
			t.Fatalf("unexpected metadata: %+v", params.Metadata)
		}
		for key, value := range expected {
			if params.Metadata[key] != value {
				t.Fatalf("incorrect metadata %s", key)
			}
		}

		return validProvisionAccount(), nil
	})

	id, err := provisioner.Provision(
		context.Background(),
		provisionActor,
		provisionDriver,
		provisionCompany,
	)
	if err != nil || id != provisionAccount || calls != 1 {
		t.Fatalf("provisioning failed: %q, %v, calls=%d", id, err, calls)
	}
}

func TestStripeProvisioningFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		change func(*stripe.Account)
	}{
		{"wrong_country", func(a *stripe.Account) {
			a.Country = "US"
		}},
		{"wrong_currency", func(a *stripe.Account) {
			a.DefaultCurrency = stripe.CurrencyUSD
		}},
		{"wrong_type", func(a *stripe.Account) {
			a.Type = stripe.AccountTypeStandard
		}},
		{"deleted", func(a *stripe.Account) {
			a.Deleted = true
		}},
		{"invalid_account_id", func(a *stripe.Account) {
			a.ID = "invalid"
		}},
		{"missing_account_id", func(a *stripe.Account) {
			a.ID = ""
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provisioner := NewAccountProvisioner(func(
				context.Context, *stripe.AccountParams,
			) (*stripe.Account, error) {
				account := validProvisionAccount()
				tc.change(account)
				return account, nil
			})

			id, err := provisioner.Provision(
				context.Background(),
				provisionActor,
				provisionDriver,
				provisionCompany,
			)
			if id != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf("invalid account accepted: %q, %v", id, err)
			}
		})
	}
}

func TestStripeProvisioningRejectsUnsafeInputs(t *testing.T) {
	calls := 0
	create := func(
		context.Context, *stripe.AccountParams,
	) (*stripe.Account, error) {
		calls++
		return validProvisionAccount(), nil
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name    string
		ctx     context.Context
		actor   string
		driver  string
		company string
	}{
		{"nil_context", nil, provisionActor, provisionDriver, provisionCompany},
		{"canceled", canceled, provisionActor, provisionDriver, provisionCompany},
		{"invalid_actor", context.Background(), "invalid", provisionDriver, provisionCompany},
		{"invalid_driver", context.Background(), provisionActor, "invalid", provisionCompany},
		{"invalid_company", context.Background(), provisionActor, provisionDriver, "invalid"},
		{"nil_uuid", context.Background(), "00000000-0000-0000-0000-000000000000", provisionDriver, provisionCompany},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := NewAccountProvisioner(create).Provision(
				tc.ctx, tc.actor, tc.driver, tc.company,
			)
			if id != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf("unsafe input accepted: %q, %v", id, err)
			}
		})
	}

	if calls != 0 {
		t.Fatalf("unsafe inputs triggered %d creations", calls)
	}

	var nilProvisioner *AccountProvisioner
	id, err := nilProvisioner.Provision(
		context.Background(),
		provisionActor,
		provisionDriver,
		provisionCompany,
	)
	if id != "" || !errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatal("nil provisioner accepted")
	}

	id, err = NewAccountProvisioner(nil).Provision(
		context.Background(),
		provisionActor,
		provisionDriver,
		provisionCompany,
	)
	if id != "" || !errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatal("missing creation dependency accepted")
	}
}

func TestStripeProvisioningRejectsProviderFailure(t *testing.T) {
	providers := []struct {
		name   string
		create AccountCreateFunc
	}{
		{
			"error",
			func(context.Context, *stripe.AccountParams) (*stripe.Account, error) {
				return nil, errors.New("Stripe unavailable")
			},
		},
		{
			"nil_response",
			func(context.Context, *stripe.AccountParams) (*stripe.Account, error) {
				return nil, nil
			},
		},
	}

	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			id, err := NewAccountProvisioner(tc.create).Provision(
				context.Background(),
				provisionActor,
				provisionDriver,
				provisionCompany,
			)
			if id != "" ||
				!errors.Is(err, ErrStripeProvisioningDenied) {
				t.Fatalf("provider failure accepted: %q, %v", id, err)
			}
		})
	}
}
