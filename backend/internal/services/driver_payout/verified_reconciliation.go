package driver_payout

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

var (
	ErrAuthoritativeVerificationUnavailable = errors.New(
		"authoritative payout verification unavailable",
	)

	ErrAuthoritativeVerificationFailed = errors.New(
		"authoritative payout verification failed",
	)
)

// VerifiedReconciliationService independently verifies an existing
// provider transfer. It never initiates money movement or changes
// CONNECT's financial database.
type VerifiedReconciliationService struct {
	provider payouts.TransferDetailsProvider
}

// NewVerifiedReconciliationService requires an authoritative
// transfer-details provider. A status-only provider is insufficient.
func NewVerifiedReconciliationService(
	provider payouts.TransferDetailsProvider,
) (*VerifiedReconciliationService, error) {
	if provider == nil {
		return nil, ErrAuthoritativeVerificationUnavailable
	}

	return &VerifiedReconciliationService{
		provider: provider,
	}, nil
}

// VerifyExistingTransferDetails retrieves provider-side transfer
// details and checks every field against the immutable expected
// request. A provider success status alone is never sufficient.
//
// The returned details are evidence for a separate financial
// authorization process; this function never finalizes a payout.
func (s *VerifiedReconciliationService) VerifyExistingTransferDetails(
	ctx context.Context,
	expected payouts.TransferRequest,
) (*payouts.VerifiedTransferDetails, error) {
	if s == nil || s.provider == nil || ctx == nil {
		return nil, ErrAuthoritativeVerificationUnavailable
	}

	if err := payouts.ValidateTransferRequest(expected); err != nil {
		return nil, fmt.Errorf(
			"%w: invalid expected transfer",
			ErrAuthoritativeVerificationFailed,
		)
	}

	lookup := payouts.TransferDetailsLookup{
		IdempotencyKey: expected.IdempotencyKey,
	}

	if err := payouts.ValidateTransferDetailsLookup(lookup); err != nil {
		return nil, fmt.Errorf(
			"%w: invalid transfer lookup",
			ErrAuthoritativeVerificationFailed,
		)
	}

	actual, err := s.provider.LookupTransferDetails(ctx, lookup)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: provider details lookup: %v",
			ErrAuthoritativeVerificationFailed,
			err,
		)
	}

	if err := payouts.VerifyTransferDetails(expected, actual); err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			ErrAuthoritativeVerificationFailed,
			err,
		)
	}

	return &actual, nil
}
