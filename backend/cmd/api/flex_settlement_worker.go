package main

import (
	"errors"
	"fmt"
	"strings"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	"github.com/jackc/pgx/v5/pgxpool"
)

// flexSettlementWorkerDependencies contains the independently
// constructed financial dependencies required by discovery.
type flexSettlementWorkerDependencies struct {
	Verifier stripepayment.FlexCaptureVerifier
	Capture  stripepayment.FlexCaptureExecutor
}

// newFlexSettlementWorkerDependencies constructs Stripe clients
// without submitting financial operations or making network calls.
func newFlexSettlementWorkerDependencies(
	secretKey string,
) (*flexSettlementWorkerDependencies, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, errors.New(
			"FLEX settlement worker requires Stripe secret key",
		)
	}

	executor, err := stripepayment.NewExecutor(secretKey)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX verification executor: %w",
			err,
		)
	}

	// The public Executor interface only exposes Execute.
	// Verify the additional recovery capability explicitly.
	verifier, ok := executor.(stripepayment.FlexCaptureVerifier)
	if !ok {
		return nil, errors.New(
			"Stripe executor does not support FLEX capture verification",
		)
	}

	capture, err := stripepayment.NewFlexCaptureAdapter(secretKey)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX capture adapter: %w",
			err,
		)
	}

	return &flexSettlementWorkerDependencies{
		Verifier: verifier,
		Capture:  capture,
	}, nil
}

// buildFlexSettlementWorker constructs the existing guarded
// discovery pipeline. It does not start background execution.
func buildFlexSettlementWorker(
	db *pgxpool.Pool,
	dependencies *flexSettlementWorkerDependencies,
	batchSize int,
) (*flexsettlement.DiscoveryLifecycle, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX settlement worker requires database",
		)
	}

	if dependencies == nil ||
		dependencies.Verifier == nil ||
		dependencies.Capture == nil {
		return nil, errors.New(
			"FLEX settlement worker requires capture and verifier",
		)
	}

	recovery := flexsettlement.NewRecoveryService(
		db,
		dependencies.Verifier,
	)

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		recovery,
	)

	scheduler, err := flexsettlement.NewDiscoveryScheduler(
		db,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX discovery scheduler: %w",
			err,
		)
	}

	dispatcher, err := flexsettlement.NewDiscoveryDispatcher(
		orchestrator,
		dependencies.Capture,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX discovery dispatcher: %w",
			err,
		)
	}

	worker, err := flexsettlement.NewDiscoveryWorker(
		scheduler,
		dispatcher,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX discovery worker: %w",
			err,
		)
	}

	lifecycle, err := flexsettlement.NewDiscoveryLifecycle(worker)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX discovery lifecycle: %w",
			err,
		)
	}

	return lifecycle, nil
}
