package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type dualFinancialShutdownWorker struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
	err     error
	once    sync.Once
}

func newDualFinancialShutdownWorker(
	err error,
) *dualFinancialShutdownWorker {
	return &dualFinancialShutdownWorker{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
		err:     err,
	}
}

func (w *dualFinancialShutdownWorker) StopAndWait(
	ctx context.Context,
) error {
	w.once.Do(func() {
		close(w.started)
	})

	select {
	case <-w.release:
		closeOnceDualFinancial(w.done)
		return w.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func closeOnceDualFinancial(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func dualFinancialShutdownDependencies(
	settlement flexShutdownWaiter,
	recovery flexShutdownWaiter,
	operationalStopped chan struct{},
) shutdownDependencies {
	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})

	close(redispatchDone)
	close(presenceDone)

	return shutdownDependencies{
		HTTPServer: &http.Server{},

		Emergency:   true,
		HTTPTimeout: time.Second,

		FlexWorker:  settlement,
		FlexTimeout: 20 * time.Millisecond,

		RecoveryWorker:  recovery,
		RecoveryTimeout: 20 * time.Millisecond,

		CancelRedispatch: func() {
			closeOnceDualFinancial(operationalStopped)
		},
		RedispatchDone: redispatchDone,

		CancelStalePresence: func() {},
		StalePresenceDone:   presenceDone,
	}
}

func TestDualFinancialShutdownWaitsForBothWorkers(
	t *testing.T,
) {
	settlement := newDualFinancialShutdownWorker(nil)
	recovery := newDualFinancialShutdownWorker(nil)
	operationalStopped := make(chan struct{})

	result := make(chan error, 1)

	go func() {
		result <- coordinateShutdown(
			dualFinancialShutdownDependencies(
				settlement,
				recovery,
				operationalStopped,
			),
		)
	}()

	select {
	case <-settlement.started:
	case <-time.After(2 * time.Second):
		t.Fatal("settlement shutdown did not start")
	}

	select {
	case <-operationalStopped:
		t.Fatal("operational workers stopped before financial completion")
	default:
	}

	close(settlement.release)

	select {
	case <-recovery.started:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery shutdown did not start")
	}

	select {
	case <-operationalStopped:
		t.Fatal("operational workers stopped before recovery completion")
	default:
	}

	// The recovery deadline expires, but the coordinator must
	// continue waiting for actual worker completion.
	time.Sleep(50 * time.Millisecond)

	select {
	case err := <-result:
		t.Fatalf("shutdown returned before recovery completed: %v", err)
	default:
	}

	close(recovery.release)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not complete")
	}

	select {
	case <-operationalStopped:
	default:
		t.Fatal("operational cleanup was not executed")
	}
}

func TestDualFinancialShutdownPreservesBothErrors(
	t *testing.T,
) {
	settlementErr := errors.New("settlement shutdown failure")
	recoveryErr := errors.New("recovery shutdown failure")

	settlement := newDualFinancialShutdownWorker(settlementErr)
	recovery := newDualFinancialShutdownWorker(recoveryErr)

	close(settlement.release)
	close(recovery.release)

	err := coordinateShutdown(
		dualFinancialShutdownDependencies(
			settlement,
			recovery,
			make(chan struct{}),
		),
	)

	if err == nil {
		t.Fatal("expected financial shutdown errors")
	}

	if !errors.Is(err, settlementErr) {
		t.Fatalf("settlement error lost: %v", err)
	}

	if !errors.Is(err, recoveryErr) {
		t.Fatalf("recovery error lost: %v", err)
	}

	if !strings.Contains(err.Error(), "authorization recovery") {
		t.Fatalf("recovery error context missing: %v", err)
	}
}

func TestDualFinancialShutdownWithoutRecovery(
	t *testing.T,
) {
	settlement := newDualFinancialShutdownWorker(nil)
	close(settlement.release)

	operationalStopped := make(chan struct{})

	err := coordinateShutdown(
		dualFinancialShutdownDependencies(
			settlement,
			nil,
			operationalStopped,
		),
	)

	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-operationalStopped:
	default:
		t.Fatal("operational cleanup did not complete")
	}
}
