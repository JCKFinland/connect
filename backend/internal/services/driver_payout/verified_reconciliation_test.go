package driver_payout

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

type verifiedReconciliationProvider struct {
	details payouts.VerifiedTransferDetails
	err     error
	calls   int
	key     string
}

func (p *verifiedReconciliationProvider) LookupTransferDetails(
	_ context.Context,
	lookup payouts.TransferDetailsLookup,
) (payouts.VerifiedTransferDetails, error) {
	p.calls++
	p.key = lookup.IdempotencyKey
	return p.details, p.err
}

var _ payouts.TransferDetailsProvider = (*verifiedReconciliationProvider)(nil)

func expectedVerifiedReconciliationRequest() payouts.TransferRequest {
	return payouts.TransferRequest{
		PayoutID:       "payout-123",
		IdempotencyKey: "immutable-key-123",
		DestinationID:  "destination-123",
		AmountCents:    9300,
		Currency:       "EUR",
	}
}

func successfulVerifiedReconciliationDetails(
	expected payouts.TransferRequest,
) payouts.VerifiedTransferDetails {
	return payouts.VerifiedTransferDetails{
		Status:            payouts.TransferSucceeded,
		PayoutID:          expected.PayoutID,
		IdempotencyKey:    expected.IdempotencyKey,
		DestinationID:     expected.DestinationID,
		AmountCents:       expected.AmountCents,
		Currency:          expected.Currency,
		ProviderReference: "provider-reference-123",
	}
}

func TestVerifiedReconciliationSuccess(t *testing.T) {
	expected := expectedVerifiedReconciliationRequest()
	provider := &verifiedReconciliationProvider{
		details: successfulVerifiedReconciliationDetails(expected),
	}

	service, err := NewVerifiedReconciliationService(provider)
	if err != nil {
		t.Fatal(err)
	}

	actual, err := service.VerifyExistingTransferDetails(
		context.Background(),
		expected,
	)
	if err != nil {
		t.Fatal(err)
	}

	if actual == nil ||
		actual.ProviderReference != "provider-reference-123" ||
		provider.calls != 1 ||
		provider.key != expected.IdempotencyKey {
		t.Fatalf(
			"unexpected verification result: actual=%+v calls=%d key=%q",
			actual, provider.calls, provider.key,
		)
	}
}

func TestVerifiedReconciliationRejectsMismatch(t *testing.T) {
	expected := expectedVerifiedReconciliationRequest()

	cases := []struct {
		name   string
		mutate func(*payouts.VerifiedTransferDetails)
	}{
		{
			name: "wrong_payout",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.PayoutID = "other-payout"
			},
		},
		{
			name: "wrong_key",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.IdempotencyKey = "other-key"
			},
		},
		{
			name: "wrong_destination",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.DestinationID = "other-destination"
			},
		},
		{
			name: "wrong_amount",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.AmountCents++
			},
		},
		{
			name: "wrong_currency",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.Currency = "USD"
			},
		},
		{
			name: "pending_status",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.Status = payouts.TransferPending
			},
		},
		{
			name: "missing_reference",
			mutate: func(d *payouts.VerifiedTransferDetails) {
				d.ProviderReference = ""
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			details := successfulVerifiedReconciliationDetails(expected)
			tc.mutate(&details)

			provider := &verifiedReconciliationProvider{
				details: details,
			}

			service, err := NewVerifiedReconciliationService(provider)
			if err != nil {
				t.Fatal(err)
			}

			result, err := service.VerifyExistingTransferDetails(
				context.Background(),
				expected,
			)

			if !errors.Is(
				err,
				ErrAuthoritativeVerificationFailed,
			) || result != nil {
				t.Fatalf(
					"mismatched transfer accepted: result=%+v err=%v",
					result, err,
				)
			}

			if provider.calls != 1 {
				t.Fatalf("unexpected lookup count: %d", provider.calls)
			}
		})
	}
}

func TestVerifiedReconciliationProviderFailure(t *testing.T) {
	expected := expectedVerifiedReconciliationRequest()
	provider := &verifiedReconciliationProvider{
		err: errors.New("provider unavailable"),
	}

	service, err := NewVerifiedReconciliationService(provider)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.VerifyExistingTransferDetails(
		context.Background(),
		expected,
	)

	if !errors.Is(err, ErrAuthoritativeVerificationFailed) ||
		result != nil ||
		provider.calls != 1 {
		t.Fatalf(
			"provider failure accepted: result=%+v err=%v calls=%d",
			result, err, provider.calls,
		)
	}
}

func TestVerifiedReconciliationInvalidRequestFailsBeforeLookup(
	t *testing.T,
) {
	provider := &verifiedReconciliationProvider{}
	service, err := NewVerifiedReconciliationService(provider)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.VerifyExistingTransferDetails(
		context.Background(),
		payouts.TransferRequest{},
	)

	if !errors.Is(err, ErrAuthoritativeVerificationFailed) ||
		result != nil ||
		provider.calls != 0 {
		t.Fatalf(
			"invalid request reached provider: result=%+v err=%v calls=%d",
			result, err, provider.calls,
		)
	}
}

func TestVerifiedReconciliationUnavailable(t *testing.T) {
	_, err := NewVerifiedReconciliationService(nil)
	if !errors.Is(err, ErrAuthoritativeVerificationUnavailable) {
		t.Fatalf("nil provider accepted: %v", err)
	}

	var service *VerifiedReconciliationService

	result, err := service.VerifyExistingTransferDetails(
		context.Background(),
		expectedVerifiedReconciliationRequest(),
	)

	if !errors.Is(err, ErrAuthoritativeVerificationUnavailable) ||
		result != nil {
		t.Fatalf(
			"nil service accepted: result=%+v err=%v",
			result, err,
		)
	}
}
