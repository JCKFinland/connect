package flex_settlement

import (
	"context"
	"errors"
	"fmt"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	stripego "github.com/stripe/stripe-go/v86"
)

var ErrCaptureOutcomeUnresolved = errors.New(
	"FLEX capture outcome is unresolved; provider reconciliation required",
)

var ErrCaptureAlreadyRequested = errors.New(
	"FLEX capture was previously requested; provider reconciliation required",
)

// ExecuteCapture submits at most one new provider capture request.
//
// It holds the settlement execution lock throughout recovery,
// preparation, marking, and the provider call.
//
// A durable capture-request marker prevents blind retries after
// timeouts, process crashes, or unresolved provider outcomes.
func (o *Orchestrator) ExecuteCapture(
	ctx context.Context,
	settlementID string,
	executor stripepayment.FlexCaptureExecutor,
) (*RecoveryResult, error) {
	if o == nil ||
		o.db == nil ||
		o.guard == nil ||
		o.recovery == nil ||
		executor == nil {
		return nil, errors.New(
			"FLEX capture execution dependencies are not configured",
		)
	}

	var result *RecoveryResult

	err := o.guard.WithSettlementLock(
		ctx,
		settlementID,
		func(lockedCtx context.Context) error {
			recovered, err := o.recovery.Recover(
				lockedCtx,
				settlementID,
			)
			if err != nil {
				return fmt.Errorf(
					"recover before FLEX capture execution: %w",
					err,
				)
			}

			if recovered == nil {
				return errors.New(
					"FLEX recovery returned no result",
				)
			}

			switch recovered.Status {
			case RecoveryAlreadyFinalized, RecoveryCancelled:
				result = recovered
				return nil

			case RecoveryCaptured:
				// Provider capture already occurred. Reconciliation,
				// not execution, must finalize local records.
				return ErrCaptureRequiresExecution

			case RecoveryPendingProvider:
				// Recovery reports PENDING_PROVIDER for an initially
				// PENDING settlement, even when Stripe confirms that
				// the authorization remains capturable.
				//
				// Only requires_capture permits first execution.
				if recovered.ProviderStatus !=
					stripego.PaymentIntentStatusRequiresCapture {
					return ErrCaptureOutcomeUnresolved
				}

			case RecoveryReadyForCapture:
				// The provider reports an existing manual
				// authorization that remains capturable.

			default:
				return postgres.ErrFlexSettlementInvalidState
			}

			settlement, _, err :=
				postgres.ClaimFlexSettlementProcessing(
					lockedCtx,
					o.db,
					settlementID,
				)
			if err != nil {
				return fmt.Errorf(
					"claim FLEX capture execution: %w",
					err,
				)
			}

			if settlement == nil ||
				settlement.Status != "PROCESSING" {
				return postgres.ErrFlexSettlementInvalidState
			}

			// Check the durable marker before preparing any further
			// local financial records.
			var previouslyRequested bool

			err = o.db.QueryRow(lockedCtx, `
				SELECT capture_requested_at IS NOT NULL
				FROM flex_settlements
				WHERE id = $1
			`, settlementID).Scan(&previouslyRequested)
			if err != nil {
				return fmt.Errorf(
					"read FLEX capture execution marker: %w",
					err,
				)
			}

			if previouslyRequested {
				return ErrCaptureAlreadyRequested
			}

			prepared, err :=
				postgres.PrepareFlexSettlementCapture(
					lockedCtx,
					o.db,
					settlementID,
				)
			if err != nil {
				return fmt.Errorf(
					"prepare FLEX capture execution: %w",
					err,
				)
			}

			if prepared == nil ||
				prepared.ProviderAuthorizationID == "" ||
				prepared.CaptureIdempotencyKey == "" {
				return postgres.ErrFlexSettlementInvalidState
			}

			marked, err :=
				postgres.MarkFlexSettlementCaptureRequested(
					lockedCtx,
					o.db,
					settlementID,
				)
			if errors.Is(err, postgres.ErrFlexCaptureAlreadyRequested) {
				return ErrCaptureAlreadyRequested
			}
			if err != nil {
				return fmt.Errorf(
					"persist FLEX capture execution marker: %w",
					err,
				)
			}

			if marked == nil ||
				marked.Status != "PROCESSING" ||
				marked.ProviderAuthorizationID !=
					prepared.ProviderAuthorizationID ||
				marked.CaptureIdempotencyKey !=
					prepared.CaptureIdempotencyKey {
				return postgres.ErrFlexSettlementInvalidState
			}

			captured, err := executor.ExecuteFlexCapture(
				lockedCtx,
				stripepayment.FlexCaptureRequest{
					ProviderAuthorizationID: marked.ProviderAuthorizationID,
					AmountCents:             marked.AmountCents,
					Currency:                marked.Currency,
					CaptureIdempotencyKey:   marked.CaptureIdempotencyKey,
				},
			)
			if err != nil {
				return fmt.Errorf(
					"%w: %v",
					ErrCaptureOutcomeUnresolved,
					err,
				)
			}

			if captured == nil {
				return ErrCaptureOutcomeUnresolved
			}

			if captured.ProviderStatus !=
				stripego.PaymentIntentStatusSucceeded {
				return ErrCaptureOutcomeUnresolved
			}

			if captured.ProviderCaptureID !=
				marked.ProviderAuthorizationID {
				return ErrCaptureOutcomeUnresolved
			}

			// A successful executor response is not sufficient
			// evidence for financial finalization. Independently
			// verify authoritative provider state.
			verified, err := o.recovery.Recover(
				lockedCtx,
				settlementID,
			)
			if err != nil {
				return fmt.Errorf(
					"%w: post-capture verification: %v",
					ErrCaptureOutcomeUnresolved,
					err,
				)
			}

			if verified == nil ||
				verified.Status != RecoveryCaptured ||
				verified.ProviderStatus !=
					stripego.PaymentIntentStatusSucceeded ||
				verified.ProviderCaptureID !=
					marked.ProviderAuthorizationID {
				return ErrCaptureOutcomeUnresolved
			}

			if err := postgres.FinalizeFlexSettlementCapture(
				lockedCtx,
				o.db,
				settlementID,
				verified.ProviderCaptureID,
			); err != nil {
				return fmt.Errorf(
					"finalize verified FLEX capture: %w",
					err,
				)
			}

			result = verified

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}
