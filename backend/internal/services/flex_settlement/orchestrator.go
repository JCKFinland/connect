package flex_settlement

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCaptureRequiresExecution = errors.New(
	"FLEX settlement requires a separately authorized capture execution",
)

type Orchestrator struct {
	guard    *ExecutionGuard
	recovery *RecoveryService
	db       *pgxpool.Pool
}

func NewOrchestrator(
	db *pgxpool.Pool,
	recovery *RecoveryService,
) *Orchestrator {
	return &Orchestrator{
		guard:    NewExecutionGuard(db),
		recovery: recovery,
		db:       db,
	}
}

// Reconcile resolves existing provider state under the settlement
// execution lock. It never submits a new Stripe capture request.
func (o *Orchestrator) Reconcile(
	ctx context.Context,
	settlementID string,
) (*RecoveryResult, error) {
	if o == nil ||
		o.db == nil ||
		o.guard == nil ||
		o.recovery == nil {
		return nil, errors.New(
			"FLEX settlement orchestrator is not configured",
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
					"recover FLEX capture: %w",
					err,
				)
			}

			if recovered == nil {
				return errors.New(
					"FLEX recovery returned no result",
				)
			}

			switch recovered.Status {
			case RecoveryCaptured:
				if recovered.ProviderCaptureID == "" {
					return errors.New(
						"verified FLEX capture has no provider identity",
					)
				}

				settlement, _, err := postgres.ClaimFlexSettlementProcessing(
					lockedCtx,
					o.db,
					settlementID,
				)
				if err != nil {
					return fmt.Errorf(
						"claim verified FLEX settlement: %w",
						err,
					)
				}

				if settlement == nil || settlement.Status != "PROCESSING" {
					return postgres.ErrFlexSettlementInvalidState
				}

				if _, err := postgres.PrepareFlexSettlementCapture(
					lockedCtx,
					o.db,
					settlementID,
				); err != nil {
					return fmt.Errorf(
						"prepare verified FLEX capture: %w",
						err,
					)
				}

				if err := postgres.FinalizeFlexSettlementCapture(
					lockedCtx,
					o.db,
					settlementID,
					recovered.ProviderCaptureID,
				); err != nil {
					return fmt.Errorf(
						"finalize verified FLEX capture: %w",
						err,
					)
				}

			case RecoveryReadyForCapture:
				// A separate execution workflow will decide whether
				// a new provider capture request is safe.
				// This reconciliation method never captures funds.

			case RecoveryPendingProvider,
				RecoveryCancelled,
				RecoveryAlreadyFinalized:
				// No local financial finalization is permitted.

			default:
				return errors.New(
					"unsupported FLEX recovery state",
				)
			}

			result = recovered
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}
