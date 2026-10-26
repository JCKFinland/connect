package payouts

import (
	"errors"
	"strings"
)

var ErrTransferVerificationFailed = errors.New(
	"payout transfer verification failed",
)

// VerifiedTransferDetails represents authoritative provider-retrieved
// transfer details, not an unverified initiation response.
type VerifiedTransferDetails struct {
	Status            TransferStatus
	PayoutID          string
	IdempotencyKey    string
	DestinationID     string
	AmountCents       int64
	Currency          string
	ProviderReference string
}

// VerifyTransferDetails compares independently retrieved provider
// transfer data against the expected immutable transfer request.
//
// This function performs no external calls or database operations.
func VerifyTransferDetails(
	expected TransferRequest,
	actual VerifiedTransferDetails,
) error {
	if err := ValidateTransferRequest(expected); err != nil {
		return ErrTransferVerificationFailed
	}

	if actual.Status != TransferSucceeded ||
		actual.PayoutID != expected.PayoutID ||
		actual.IdempotencyKey != expected.IdempotencyKey ||
		actual.DestinationID != expected.DestinationID ||
		actual.AmountCents != expected.AmountCents ||
		actual.Currency != expected.Currency ||
		strings.TrimSpace(actual.ProviderReference) == "" ||
		actual.ProviderReference != strings.TrimSpace(
			actual.ProviderReference,
		) ||
		len(actual.ProviderReference) > 255 {
		return ErrTransferVerificationFailed
	}

	return nil
}
