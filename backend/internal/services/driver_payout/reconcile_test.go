package driver_payout

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

type fakeProvider struct {
	result        payouts.TransferResult
	err           error
	lookupCalls   int
	initiateCalls int
	lookupKey     string
}

func (f *fakeProvider) InitiateTransfer(
	context.Context,
	payouts.TransferRequest,
) (payouts.TransferResult, error) {
	f.initiateCalls++
	return payouts.TransferResult{}, errors.New(
		"initiation forbidden in reconciliation",
	)
}

func (f *fakeProvider) LookupTransfer(
	_ context.Context,
	req payouts.TransferLookup,
) (payouts.TransferResult, error) {
	f.lookupCalls++
	f.lookupKey = req.IdempotencyKey
	return f.result, f.err
}

func TestReconcileVerifiedSuccess(t *testing.T) {
	provider := &fakeProvider{
		result: payouts.TransferResult{
			Status:            payouts.TransferSucceeded,
			ProviderReference: "synthetic-provider-transfer",
		},
	}

	service, err := NewReconciler(provider)
	if err != nil {
		t.Fatal(err)
	}

	verified, err := service.VerifyExistingTransfer(
		context.Background(),
		ReconciliationRequest{
			PayoutID:       "synthetic-payout",
			IdempotencyKey: "synthetic-key",
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if verified == nil ||
		verified.ProviderReference != "synthetic-provider-transfer" ||
		verified.PayoutID != "synthetic-payout" ||
		verified.IdempotencyKey != "synthetic-key" {
		t.Fatalf("unexpected verified transfer: %+v", verified)
	}

	if provider.lookupCalls != 1 ||
		provider.initiateCalls != 0 ||
		provider.lookupKey != "synthetic-key" {
		t.Fatal("unexpected provider operations")
	}
}

func TestReconcileUnresolvedOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		result payouts.TransferResult
		err    error
	}{
		{
			name: "pending",
			result: payouts.TransferResult{
				Status: payouts.TransferPending,
			},
		},
		{
			name: "failed",
			result: payouts.TransferResult{
				Status: payouts.TransferFailed,
			},
		},
		{
			name: "unknown",
			result: payouts.TransferResult{
				Status: payouts.TransferUnknown,
			},
		},
		{
			name: "provider error",
			err:  errors.New("synthetic timeout"),
		},
		{
			name: "invalid success",
			result: payouts.TransferResult{
				Status: payouts.TransferSucceeded,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeProvider{
				result: tc.result,
				err:    tc.err,
			}

			service, err := NewReconciler(provider)
			if err != nil {
				t.Fatal(err)
			}

			verified, err := service.VerifyExistingTransfer(
				context.Background(),
				ReconciliationRequest{
					PayoutID:       "synthetic-payout",
					IdempotencyKey: "synthetic-key",
				},
			)

			if verified != nil ||
				!errors.Is(err, ErrPayoutUnresolved) {
				t.Fatalf(
					"expected unresolved, got %+v %v",
					verified,
					err,
				)
			}

			if provider.lookupCalls != 1 ||
				provider.initiateCalls != 0 {
				t.Fatal("unexpected provider operations")
			}
		})
	}
}

func TestReconcileRejectsInvalidInput(t *testing.T) {
	provider := &fakeProvider{}

	service, err := NewReconciler(provider)
	if err != nil {
		t.Fatal(err)
	}

	for _, req := range []ReconciliationRequest{
		{},
		{PayoutID: "synthetic-payout"},
		{IdempotencyKey: "synthetic-key"},
	} {
		verified, err := service.VerifyExistingTransfer(
			context.Background(),
			req,
		)

		if verified != nil ||
			!errors.Is(err, ErrInvalidReconciliation) {
			t.Fatalf("invalid request accepted: %+v", req)
		}
	}

	if provider.lookupCalls != 0 ||
		provider.initiateCalls != 0 {
		t.Fatal("invalid request contacted provider")
	}
}

func TestReconcilerRejectsMissingProvider(t *testing.T) {
	service, err := NewReconciler(nil)

	if service != nil ||
		!errors.Is(err, ErrInvalidReconciliation) {
		t.Fatal("nil provider accepted")
	}
}
