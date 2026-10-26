package driver_payout

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

var (
	ErrInvalidReconciliation = errors.New(
		"invalid driver payout reconciliation",
	)
	ErrPayoutUnresolved = errors.New(
		"driver payout provider outcome unresolved",
	)
)

type ReconciliationRequest struct {
	PayoutID       string
	IdempotencyKey string
}

type VerifiedTransfer struct {
	PayoutID          string
	IdempotencyKey    string
	ProviderReference string
}

type Reconciler struct {
	provider payouts.Provider
}

func NewReconciler(provider payouts.Provider) (*Reconciler, error) {
	if provider == nil {
		return nil, ErrInvalidReconciliation
	}

	return &Reconciler{provider: provider}, nil
}

// VerifyExistingTransfer performs provider lookup only.
//
// It never initiates a transfer and never changes database state.
//
// The caller must independently match the returned provider reference
// to the authoritative payout identity, amount, currency and destination
// before any financial finalization.
func (r *Reconciler) VerifyExistingTransfer(
	ctx context.Context,
	req ReconciliationRequest,
) (*VerifiedTransfer, error) {
	if r == nil || r.provider == nil {
		return nil, ErrInvalidReconciliation
	}

	if req.PayoutID == "" ||
		payouts.ValidateTransferLookup(payouts.TransferLookup{
			IdempotencyKey: req.IdempotencyKey,
		}) != nil {
		return nil, ErrInvalidReconciliation
	}

	result, err := r.provider.LookupTransfer(
		ctx,
		payouts.TransferLookup{
			IdempotencyKey: req.IdempotencyKey,
		},
	)

	if err != nil {
		return nil, fmt.Errorf(
			"%w: provider lookup: %v",
			ErrPayoutUnresolved,
			err,
		)
	}

	if err := payouts.ValidateTransferResult(result); err != nil {
		return nil, fmt.Errorf(
			"%w: invalid provider response",
			ErrPayoutUnresolved,
		)
	}

	if result.Status != payouts.TransferSucceeded {
		return nil, ErrPayoutUnresolved
	}

	return &VerifiedTransfer{
		PayoutID:          req.PayoutID,
		IdempotencyKey:    req.IdempotencyKey,
		ProviderReference: result.ProviderReference,
	}, nil
}
