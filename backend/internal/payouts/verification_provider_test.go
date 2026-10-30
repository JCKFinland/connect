package payouts

import (
	"context"
	"errors"
	"testing"
)

type testTransferDetailsProvider struct {
	details VerifiedTransferDetails
}

func (p *testTransferDetailsProvider) LookupTransferDetails(
	_ context.Context,
	lookup TransferDetailsLookup,
) (VerifiedTransferDetails, error) {
	if err := ValidateTransferDetailsLookup(lookup); err != nil {
		return VerifiedTransferDetails{}, err
	}

	return p.details, nil
}

var _ TransferDetailsProvider = (*testTransferDetailsProvider)(nil)

func TestTransferDetailsLookupValidation(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		valid bool
	}{
		{"valid", "payout-idempotency-key", true},
		{"empty", "", false},
		{"whitespace", " payout-key ", false},
		{"spaces_only", "   ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTransferDetailsLookup(
				TransferDetailsLookup{
					IdempotencyKey: tc.key,
				},
			)

			if tc.valid && err != nil {
				t.Fatalf("valid lookup rejected: %v", err)
			}

			if !tc.valid && !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("expected invalid request, got %v", err)
			}
		})
	}
}

func TestTransferDetailsProviderContract(t *testing.T) {
	expected := TransferRequest{
		PayoutID:       "payout-123",
		IdempotencyKey: "key-123",
		DestinationID:  "destination-123",
		AmountCents:    9300,
		Currency:       "EUR",
	}

	provider := &testTransferDetailsProvider{
		details: VerifiedTransferDetails{
			Status:            TransferSucceeded,
			PayoutID:          expected.PayoutID,
			IdempotencyKey:    expected.IdempotencyKey,
			DestinationID:     expected.DestinationID,
			AmountCents:       expected.AmountCents,
			Currency:          expected.Currency,
			ProviderReference: "provider-transfer-123",
		},
	}

	actual, err := provider.LookupTransferDetails(
		context.Background(),
		TransferDetailsLookup{
			IdempotencyKey: expected.IdempotencyKey,
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if err := VerifyTransferDetails(expected, actual); err != nil {
		t.Fatalf("authoritative transfer rejected: %v", err)
	}

	actual.AmountCents++

	if err := VerifyTransferDetails(expected, actual); !errors.Is(
		err,
		ErrTransferVerificationFailed,
	) {
		t.Fatalf("mismatched amount accepted: %v", err)
	}
}
