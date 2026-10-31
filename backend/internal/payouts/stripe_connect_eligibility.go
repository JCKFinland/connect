package payouts

import (
	"context"
	"errors"
)

var ErrStripeConnectAccountNotEligible = errors.New(
	"Stripe Connect account is not eligible",
)

// StripeConnectAccountSnapshot represents account information
// independently retrieved from Stripe.
//
// These values must not be reconstructed from CONNECT's
// expected account identity or local database records.
type StripeConnectAccountSnapshot struct {
	AccountID                 string
	Country                   string
	TransfersCapabilityActive bool
	ChargesEnabled            bool
	PayoutsEnabled            bool
	DetailsSubmitted          bool
	CurrentlyDueCount         int
	PastDueCount              int
	DisabledReason            string
}

// StripeConnectAccountReader is a read-only provider capability.
// Implementations must retrieve authoritative account details.
//
// No account creation or financial execution is permitted.
type StripeConnectAccountReader interface {
	RetrieveConnectAccount(
		context.Context,
		string,
	) (StripeConnectAccountSnapshot, error)
}

// VerifyStripeConnectAccountEligibility verifies independently
// retrieved account data against the expected identity.
//
// ChargesEnabled is intentionally not required because this
// contract concerns receipt of transfers, not direct charges.
//
// PayoutsEnabled is required by CONNECT's current policy,
// but does not guarantee a particular bank payout will succeed.
//
// This function performs no external calls or DB operations.
func VerifyStripeConnectAccountEligibility(
	expected StripeConnectAccountIdentity,
	actual StripeConnectAccountSnapshot,
) error {
	if ValidateStripeConnectAccountIdentity(expected) != nil {
		return ErrStripeConnectAccountNotEligible
	}

	if actual.AccountID != expected.AccountID ||
		actual.Country != expected.Country ||
		!actual.TransfersCapabilityActive ||
		!actual.PayoutsEnabled ||
		!actual.DetailsSubmitted ||
		actual.CurrentlyDueCount != 0 ||
		actual.PastDueCount != 0 ||
		actual.DisabledReason != "" {
		return ErrStripeConnectAccountNotEligible
	}

	return nil
}

// StripeConnectEligibilityVerifier obtains independent provider
// evidence and validates it against the expected identity.
type StripeConnectEligibilityVerifier struct {
	Reader StripeConnectAccountReader
}

func (v StripeConnectEligibilityVerifier) Verify(
	ctx context.Context,
	expected StripeConnectAccountIdentity,
) error {
	if ctx == nil ||
		v.Reader == nil ||
		ValidateStripeConnectAccountIdentity(expected) != nil {
		return ErrStripeConnectAccountNotEligible
	}

	actual, err := v.Reader.RetrieveConnectAccount(
		ctx,
		expected.AccountID,
	)
	if err != nil {
		return ErrStripeConnectAccountNotEligible
	}

	return VerifyStripeConnectAccountEligibility(
		expected,
		actual,
	)
}
