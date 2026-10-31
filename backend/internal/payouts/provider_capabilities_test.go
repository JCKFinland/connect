package payouts

import (
	"context"
	"errors"
	"testing"
)

type capabilityTestProvider struct{}

func (*capabilityTestProvider) InitiateTransfer(
	context.Context,
	TransferRequest,
) (TransferResult, error) {
	return TransferResult{}, errors.New("test-only: initiation forbidden")
}

func (*capabilityTestProvider) LookupTransfer(
	context.Context,
	TransferLookup,
) (TransferResult, error) {
	return TransferResult{}, errors.New("test-only: lookup forbidden")
}

type capabilityTestDetailsProvider struct{}

func (*capabilityTestDetailsProvider) LookupTransferDetails(
	context.Context,
	TransferDetailsLookup,
) (VerifiedTransferDetails, error) {
	return VerifiedTransferDetails{},
		errors.New("test-only: details lookup forbidden")
}

func TestValidateProviderCapabilities(t *testing.T) {
	submission := &capabilityTestProvider{}
	details := &capabilityTestDetailsProvider{}

	tests := []struct {
		name string
		cap  ProviderCapabilities
		ok   bool
	}{
		{
			name: "complete capabilities",
			cap: ProviderCapabilities{
				Name:       "TEST_BANK",
				Submission: submission,
				Details:    details,
			},
			ok: true,
		},
		{
			name: "missing name",
			cap: ProviderCapabilities{
				Submission: submission,
				Details:    details,
			},
		},
		{
			name: "whitespace name",
			cap: ProviderCapabilities{
				Name:       " TEST_BANK ",
				Submission: submission,
				Details:    details,
			},
		},
		{
			name: "missing submission",
			cap: ProviderCapabilities{
				Name:    "TEST_BANK",
				Details: details,
			},
		},
		{
			name: "missing verification",
			cap: ProviderCapabilities{
				Name:       "TEST_BANK",
				Submission: submission,
			},
		},
		{
			name: "typed nil submission",
			cap: ProviderCapabilities{
				Name:       "TEST_BANK",
				Submission: (*capabilityTestProvider)(nil),
				Details:    details,
			},
		},
		{
			name: "typed nil verification",
			cap: ProviderCapabilities{
				Name:       "TEST_BANK",
				Submission: submission,
				Details:    (*capabilityTestDetailsProvider)(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderCapabilities(tt.cap)

			if tt.ok {
				if err != nil {
					t.Fatalf("expected valid capabilities: %v", err)
				}
				return
			}

			if !errors.Is(err, ErrInvalidProviderCapabilities) {
				t.Fatalf("expected capability denial, got %v", err)
			}
		})
	}
}
