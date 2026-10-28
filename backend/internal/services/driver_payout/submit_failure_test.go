package driver_payout

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/payouts"
)

func TestSubmitPayoutProviderOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		result payouts.TransferResult
		err    error
		cancel bool
	}{
		{
			name: "provider error",
			err:  errors.New("synthetic provider timeout"),
		},
		{
			name: "invalid success",
			result: payouts.TransferResult{
				Status: payouts.TransferSucceeded,
			},
		},
		{
			name: "reported success",
			result: payouts.TransferResult{
				Status:            payouts.TransferSucceeded,
				ProviderReference: "synthetic-success-reference",
			},
		},
		{
			name: "reported failure",
			result: payouts.TransferResult{
				Status: payouts.TransferFailed,
			},
		},
		{
			name: "reported unknown",
			result: payouts.TransferResult{
				Status: payouts.TransferUnknown,
			},
		},
		{
			name:   "cancelled after provider initiation",
			cancel: true,
			result: payouts.TransferResult{
				Status: payouts.TransferPending,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSubmissionFixture(t)

			provider := &submissionTestProvider{
				result: tc.result,
				err:    tc.err,
			}

			submitter, err := NewSubmitter(
				fixture.db, provider,
			)
			if err != nil {
				t.Fatal(err)
			}

			requestCtx := fixture.ctx

			if tc.cancel {
				var cancel context.CancelFunc
				requestCtx, cancel = context.WithCancel(
					fixture.ctx,
				)
				defer cancel()
				provider.cancel = cancel
			}

			err = submitter.SubmitPayout(
				requestCtx, fixture.payoutID,
			)

			if !errors.Is(err, ErrSubmissionUnresolved) {
				t.Fatalf(
					"expected unresolved submission: %v",
					err,
				)
			}

			provider.mu.Lock()
			calls := provider.calls
			requests := append(
				[]payouts.TransferRequest(nil),
				provider.requests...,
			)
			provider.mu.Unlock()

			if calls != 1 || len(requests) != 1 {
				t.Fatalf(
					"expected one initiation, got %d",
					calls,
				)
			}

			request := requests[0]

			if request.PayoutID != fixture.payoutID ||
				request.DestinationID != fixture.externalDestination ||
				request.AmountCents != 9300 ||
				request.Currency != "EUR" ||
				request.IdempotencyKey == "" {
				t.Fatalf(
					"incorrect transfer request: %+v",
					request,
				)
			}

			var executionStatus string
			var payoutStatus string
			var attempts int

			err = fixture.db.QueryRow(
				fixture.ctx,
				`SELECT e.execution_status,
				        p.payout_status,
				        e.attempt_count
				   FROM driver_payout_executions e
				   JOIN driver_payouts p
				     ON p.id=e.payout_id
				  WHERE e.payout_id=$1`,
				fixture.payoutID,
			).Scan(
				&executionStatus,
				&payoutStatus,
				&attempts,
			)
			if err != nil {
				t.Fatal(err)
			}

			if executionStatus != "REQUIRES_RECONCILIATION" ||
				payoutStatus != "PENDING" ||
				attempts != 1 {
				t.Fatalf(
					"unsafe state: execution=%s payout=%s attempts=%d",
					executionStatus,
					payoutStatus,
					attempts,
				)
			}

			replayErr := submitter.SubmitPayout(
				fixture.ctx, fixture.payoutID,
			)

			if !errors.Is(
				replayErr, ErrSubmissionNotAuthorized,
			) {
				t.Fatalf(
					"replay unexpectedly authorized: %v",
					replayErr,
				)
			}

			provider.mu.Lock()
			finalCalls := provider.calls
			provider.mu.Unlock()

			if finalCalls != 1 {
				t.Fatalf(
					"provider called again: %d",
					finalCalls,
				)
			}
		})
	}
}
