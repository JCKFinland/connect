package payouts

import (
	"context"
)

// TransferDetailsLookup identifies an existing transfer for an
// authoritative provider-side details lookup.
//
// The lookup must never initiate or retry a money movement.
type TransferDetailsLookup struct {
	IdempotencyKey string
}

// TransferDetailsProvider is an optional provider capability.
//
// Implementations must retrieve transfer details independently
// from the provider's authoritative records. They must not
// construct these details from CONNECT's expected request.
//
// Existing Provider implementations remain compatible.
type TransferDetailsProvider interface {
	LookupTransferDetails(
		context.Context,
		TransferDetailsLookup,
	) (VerifiedTransferDetails, error)
}

// ValidateTransferDetailsLookup validates the immutable lookup key.
func ValidateTransferDetailsLookup(
	lookup TransferDetailsLookup,
) error {
	return ValidateTransferLookup(TransferLookup{
		IdempotencyKey: lookup.IdempotencyKey,
	})
}
