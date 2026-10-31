package payouts

import (
	"errors"
	"strings"
)

var ErrInvalidStripeConnectIdentity = errors.New(
	"invalid Stripe Connect account identity",
)

// StripeConnectAccountIdentity represents an existing Stripe
// connected account. It does not authorize account creation,
// transfers, or bank payouts.
type StripeConnectAccountIdentity struct {
	AccountID string
	Country   string
	Currency  string
}

// ValidateStripeConnectAccountIdentity validates the expected
// identity format for CONNECT's Finnish EUR payout model.
//
// This is local structural validation only. It does not prove
// ownership, onboarding completion, or payout eligibility.
func ValidateStripeConnectAccountIdentity(
	identity StripeConnectAccountIdentity,
) error {
	if !strings.HasPrefix(identity.AccountID, "acct_") {
		return ErrInvalidStripeConnectIdentity
	}

	suffix := strings.TrimPrefix(identity.AccountID, "acct_")

	if len(suffix) < 8 || len(suffix) > 64 {
		return ErrInvalidStripeConnectIdentity
	}

	for _, char := range suffix {
		if !(char >= 'a' && char <= 'z') &&
			!(char >= 'A' && char <= 'Z') &&
			!(char >= '0' && char <= '9') {
			return ErrInvalidStripeConnectIdentity
		}
	}

	if identity.Country != "FI" ||
		identity.Currency != "EUR" {
		return ErrInvalidStripeConnectIdentity
	}

	return nil
}
