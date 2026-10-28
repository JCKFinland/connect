package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type recoveryLookupProvider struct {
	lookups     int
	initiations int
	key         string
	result      payouts.TransferResult
	err         error
}

func (p *recoveryLookupProvider) InitiateTransfer(
	context.Context,
	payouts.TransferRequest,
) (payouts.TransferResult, error) {
	p.initiations++
	return payouts.TransferResult{},
		errors.New("recovery must never initiate a transfer")
}

func (p *recoveryLookupProvider) LookupTransfer(
	_ context.Context,
	lookup payouts.TransferLookup,
) (payouts.TransferResult, error) {
	p.lookups++
	p.key = lookup.IdempotencyKey
	return p.result, p.err
}

func TestRecoveryInspectorProviderOutcomes(t *testing.T) {
	cases := []struct {
		name          string
		result        payouts.TransferResult
		providerErr   error
		expectSuccess bool
	}{
		{
			name: "reported success",
			result: payouts.TransferResult{
				Status:            payouts.TransferSucceeded,
				ProviderReference: "recovery-test-reference",
			},
			expectSuccess: true,
		},
		{
			name: "pending",
			result: payouts.TransferResult{
				Status: payouts.TransferPending,
			},
		},
		{
			name: "unknown",
			result: payouts.TransferResult{
				Status: payouts.TransferUnknown,
			},
		},
		{
			name:        "provider error",
			providerErr: errors.New("synthetic lookup failure"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSubmissionFixture(t)

			execution, claimed, err :=
				postgresrepo.ClaimDriverPayoutExecution(
					fixture.ctx,
					fixture.db,
					fixture.payoutID,
				)
			if err != nil {
				t.Fatal(err)
			}
			if !claimed || execution == nil {
				t.Fatal("initial claim failed")
			}

			provider := &recoveryLookupProvider{
				result: tc.result,
				err:    tc.providerErr,
			}

			inspector, err := NewRecoveryInspector(
				fixture.db,
				provider,
			)
			if err != nil {
				t.Fatal(err)
			}

			observations, err := inspector.InspectStaleExecutions(
				fixture.ctx,
				time.Now().UTC().Add(time.Minute),
				100,
			)
			if err != nil {
				t.Fatal(err)
			}

			var matching []RecoveryObservation
			for _, observation := range observations {
				if observation.Execution.PayoutID == fixture.payoutID {
					matching = append(matching, observation)
				}
			}

			if len(matching) != 1 {
				t.Fatalf(
					"expected one observation, got %d",
					len(matching),
				)
			}

			observation := matching[0]

			if observation.Execution.IdempotencyKey !=
				execution.IdempotencyKey {
				t.Fatal("recovery identity mismatch")
			}

			if provider.initiations != 0 {
				t.Fatal("recovery initiated a transfer")
			}

			if provider.lookups != 1 {
				t.Fatalf(
					"expected one lookup, got %d",
					provider.lookups,
				)
			}

			if provider.key != execution.IdempotencyKey {
				t.Fatal("lookup used incorrect idempotency key")
			}

			if observation.LookupSucceeded != tc.expectSuccess {
				t.Fatalf(
					"unexpected lookup result: %+v",
					observation,
				)
			}

			if tc.expectSuccess {
				if observation.ProviderReference !=
					"recovery-test-reference" {
					t.Fatal("provider reference mismatch")
				}
				if observation.LookupError != nil {
					t.Fatal(observation.LookupError)
				}
			} else if observation.LookupError == nil {
				t.Fatal("unresolved lookup lacks error")
			}

			var executionStatus, payoutStatus string
			var attempts int
			var providerReference *string

			err = fixture.db.QueryRow(fixture.ctx, `
                SELECT
                    e.execution_status,
                    p.payout_status,
                    e.attempt_count,
                    e.provider_reference
                FROM driver_payout_executions e
                JOIN driver_payouts p
                  ON p.id=e.payout_id
                WHERE e.payout_id=$1
            `, fixture.payoutID).Scan(
				&executionStatus,
				&payoutStatus,
				&attempts,
				&providerReference,
			)
			if err != nil {
				t.Fatal(err)
			}

			if executionStatus != "PROCESSING" ||
				payoutStatus != "PENDING" ||
				attempts != 1 ||
				providerReference != nil {
				t.Fatalf(
					"recovery changed financial state: execution=%s payout=%s attempts=%d reference=%v",
					executionStatus,
					payoutStatus,
					attempts,
					providerReference,
				)
			}

			_, replayClaimed, err :=
				postgresrepo.ClaimDriverPayoutExecution(
					fixture.ctx,
					fixture.db,
					fixture.payoutID,
				)
			if err != nil {
				t.Fatal(err)
			}
			if replayClaimed {
				t.Fatal("recovery enabled duplicate initiation")
			}
		})
	}
}
