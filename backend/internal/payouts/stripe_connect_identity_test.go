package payouts

import (
	"errors"
	"testing"
)

func TestValidateStripeConnectAccountIdentity(t *testing.T) {
	identity := StripeConnectAccountIdentity{
		AccountID: "acct_1234567890ABCDEF",
		Country:   "FI",
		Currency:  "EUR",
	}

	if err := ValidateStripeConnectAccountIdentity(identity); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateStripeConnectAccountIdentityRejectsInvalid(
	t *testing.T,
) {
	cases := []struct {
		name     string
		identity StripeConnectAccountIdentity
	}{
		{
			name: "missing account",
			identity: StripeConnectAccountIdentity{
				Country:  "FI",
				Currency: "EUR",
			},
		},
		{
			name: "wrong prefix",
			identity: StripeConnectAccountIdentity{
				AccountID: "cus_1234567890",
				Country:   "FI",
				Currency:  "EUR",
			},
		},
		{
			name: "short account",
			identity: StripeConnectAccountIdentity{
				AccountID: "acct_123",
				Country:   "FI",
				Currency:  "EUR",
			},
		},
		{
			name: "invalid characters",
			identity: StripeConnectAccountIdentity{
				AccountID: "acct_12345678/abc",
				Country:   "FI",
				Currency:  "EUR",
			},
		},
		{
			name: "wrong country",
			identity: StripeConnectAccountIdentity{
				AccountID: "acct_1234567890",
				Country:   "SE",
				Currency:  "EUR",
			},
		},
		{
			name: "wrong currency",
			identity: StripeConnectAccountIdentity{
				AccountID: "acct_1234567890",
				Country:   "FI",
				Currency:  "USD",
			},
		},
		{
			name: "whitespace",
			identity: StripeConnectAccountIdentity{
				AccountID: " acct_1234567890",
				Country:   "FI",
				Currency:  "EUR",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStripeConnectAccountIdentity(
				tc.identity,
			)

			if !errors.Is(
				err,
				ErrInvalidStripeConnectIdentity,
			) {
				t.Fatalf(
					"expected invalid identity, got %v",
					err,
				)
			}
		})
	}
}
