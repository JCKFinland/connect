package flex_settlement

import (
	"context"
	"errors"
	"fmt"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// SettlementExecutor exposes only the existing guarded financial operations.
type SettlementExecutor interface {
	ExecuteCapture(
		context.Context,
		string,
		stripepayment.FlexCaptureExecutor,
	) (*RecoveryResult, error)

	Reconcile(
		context.Context,
		string,
	) (*RecoveryResult, error)
}

// DiscoveryDispatcher routes candidates without changing financial state.
type DiscoveryDispatcher struct {
	executor SettlementExecutor
	capture  stripepayment.FlexCaptureExecutor
}

func NewDiscoveryDispatcher(
	executor SettlementExecutor,
	capture stripepayment.FlexCaptureExecutor,
) (*DiscoveryDispatcher, error) {
	if executor == nil {
		return nil, errors.New(
			"FLEX settlement executor is required",
		)
	}

	if capture == nil {
		return nil, errors.New(
			"FLEX capture executor is required",
		)
	}

	return &DiscoveryDispatcher{
		executor: executor,
		capture:  capture,
	}, nil
}

func (d *DiscoveryDispatcher) Dispatch(
	ctx context.Context,
	candidate postgresrepo.FlexSettlementCandidate,
) (*RecoveryResult, error) {
	if d == nil || d.executor == nil || d.capture == nil {
		return nil, errors.New(
			"FLEX discovery dispatcher is not configured",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if candidate.SettlementID == "" {
		return nil, errors.New(
			"FLEX discovery candidate has no settlement ID",
		)
	}

	switch candidate.Kind {
	case postgresrepo.FlexSettlementCandidateCapture:
		return d.executor.ExecuteCapture(
			ctx,
			candidate.SettlementID,
			d.capture,
		)

	case postgresrepo.FlexSettlementCandidateRecovery:
		return d.executor.Reconcile(
			ctx,
			candidate.SettlementID,
		)

	default:
		return nil, fmt.Errorf(
			"unsupported FLEX settlement discovery category: %q",
			candidate.Kind,
		)
	}
}
