package payouts

import (
	"context"
	"errors"
	"testing"
)

// providerContractFixture contains synthetic, immutable transfer identity.
// A concrete adapter's contract test must use independent provider-side
// fixtures; it must never manufacture verification data from the expected
// request inside its LookupTransferDetails implementation.
type providerContractFixture struct {
	expected TransferRequest
	actual   VerifiedTransferDetails
}

func newProviderContractFixture() providerContractFixture {
	return providerContractFixture{
		expected: TransferRequest{
			PayoutID:       "contract-payout-123",
			IdempotencyKey: "driver-payout-execution:contract-payout-123",
			DestinationID:  "contract-destination-456",
			AmountCents:    9300,
			Currency:       "EUR",
		},
		actual: VerifiedTransferDetails{
			Status:            TransferSucceeded,
			PayoutID:          "contract-payout-123",
			IdempotencyKey:    "driver-payout-execution:contract-payout-123",
			DestinationID:     "contract-destination-456",
			AmountCents:       9300,
			Currency:          "EUR",
			ProviderReference: "contract-transfer-789",
		},
	}
}

// assertProviderVerificationContract exercises the independent details
// capability without calling InitiateTransfer or LookupTransfer.
//
// The supplied adapter must be configured with an authoritative,
// read-only test fixture before this helper is invoked.
//
// This helper validates returned evidence but cannot independently prove
// that a real provider's HTTP implementation never initiates a transfer.
// Provider-specific transport tests must establish that separately.
func assertProviderVerificationContract(
	t *testing.T,
	provider TransferDetailsProvider,
	fixture providerContractFixture,
) {
	t.Helper()

	if isNilProviderCapability(provider) {
		t.Fatal("verification provider is unavailable")
	}

	if err := ValidateTransferRequest(fixture.expected); err != nil {
		t.Fatalf("invalid expected fixture: %v", err)
	}

	lookup := TransferDetailsLookup{
		IdempotencyKey: fixture.expected.IdempotencyKey,
	}

	if err := ValidateTransferDetailsLookup(lookup); err != nil {
		t.Fatalf("invalid lookup fixture: %v", err)
	}

	actual, err := provider.LookupTransferDetails(
		context.Background(),
		lookup,
	)
	if err != nil {
		t.Fatalf("authoritative lookup failed: %v", err)
	}

	if actual != fixture.actual {
		t.Fatalf(
			"provider evidence differs from independent fixture: got %+v",
			actual,
		)
	}

	if err := VerifyTransferDetails(fixture.expected, actual); err != nil {
		t.Fatalf("valid provider evidence rejected: %v", err)
	}

	mutations := []struct {
		name   string
		mutate func(*VerifiedTransferDetails)
	}{
		{"pending", func(v *VerifiedTransferDetails) {
			v.Status = TransferPending
		}},
		{"failed", func(v *VerifiedTransferDetails) {
			v.Status = TransferFailed
		}},
		{"unknown", func(v *VerifiedTransferDetails) {
			v.Status = TransferUnknown
		}},
		{"payout mismatch", func(v *VerifiedTransferDetails) {
			v.PayoutID = "different-payout"
		}},
		{"key mismatch", func(v *VerifiedTransferDetails) {
			v.IdempotencyKey = "different-key"
		}},
		{"destination mismatch", func(v *VerifiedTransferDetails) {
			v.DestinationID = "different-destination"
		}},
		{"amount mismatch", func(v *VerifiedTransferDetails) {
			v.AmountCents++
		}},
		{"currency mismatch", func(v *VerifiedTransferDetails) {
			v.Currency = "USD"
		}},
		{"missing reference", func(v *VerifiedTransferDetails) {
			v.ProviderReference = ""
		}},
		{"whitespace reference", func(v *VerifiedTransferDetails) {
			v.ProviderReference = " invalid "
		}},
	}

	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			candidate := actual
			tc.mutate(&candidate)

			if !errors.Is(
				VerifyTransferDetails(fixture.expected, candidate),
				ErrTransferVerificationFailed,
			) {
				t.Fatalf("invalid evidence accepted: %+v", candidate)
			}
		})
	}
}

type providerContractFixtureAdapter struct {
	records map[string]VerifiedTransferDetails
	lookups []string
}

func (p *providerContractFixtureAdapter) LookupTransferDetails(
	_ context.Context,
	lookup TransferDetailsLookup,
) (VerifiedTransferDetails, error) {
	if err := ValidateTransferDetailsLookup(lookup); err != nil {
		return VerifiedTransferDetails{}, err
	}

	p.lookups = append(p.lookups, lookup.IdempotencyKey)

	record, ok := p.records[lookup.IdempotencyKey]
	if !ok {
		return VerifiedTransferDetails{}, ErrUnknownOutcome
	}

	return record, nil
}

var _ TransferDetailsProvider = (*providerContractFixtureAdapter)(nil)

func TestProviderVerificationContractHarness(t *testing.T) {
	fixture := newProviderContractFixture()

	adapter := &providerContractFixtureAdapter{
		records: map[string]VerifiedTransferDetails{
			fixture.actual.IdempotencyKey: fixture.actual,
		},
	}

	assertProviderVerificationContract(t, adapter, fixture)

	if len(adapter.lookups) != 1 ||
		adapter.lookups[0] != fixture.expected.IdempotencyKey {
		t.Fatalf("unexpected provider lookup keys: %+v", adapter.lookups)
	}
}

func TestProviderVerificationContractRejectsUnavailableProvider(
	t *testing.T,
) {
	var provider *providerContractFixtureAdapter

	if !isNilProviderCapability(provider) {
		t.Fatal("typed-nil provider accepted")
	}
}

func TestProviderVerificationContractRejectsProviderError(
	t *testing.T,
) {
	fixture := newProviderContractFixture()

	adapter := &providerContractFixtureAdapter{
		records: map[string]VerifiedTransferDetails{},
	}

	_, err := adapter.LookupTransferDetails(
		context.Background(),
		TransferDetailsLookup{
			IdempotencyKey: fixture.expected.IdempotencyKey,
		},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("missing provider record accepted: %v", err)
	}
}

func TestProviderVerificationContractRejectsInvalidLookup(
	t *testing.T,
) {
	adapter := &providerContractFixtureAdapter{
		records: map[string]VerifiedTransferDetails{},
	}

	_, err := adapter.LookupTransferDetails(
		context.Background(),
		TransferDetailsLookup{
			IdempotencyKey: " invalid ",
		},
	)

	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid lookup accepted: %v", err)
	}

	if len(adapter.lookups) != 0 {
		t.Fatal("invalid lookup reached provider records")
	}
}
