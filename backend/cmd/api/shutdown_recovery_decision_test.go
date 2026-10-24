package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestRecoveryShutdownDecisionFailure(t *testing.T) {
	signals := make(chan os.Signal)
	settlement := make(chan error)
	recovery := make(chan error, 1)
	server := make(chan error)

	failure := errors.New("recovery scheduler failed")
	recovery <- failure

	decision := waitForSupervisedShutdownDecision(
		signals,
		settlement,
		recovery,
		server,
	)

	if !decision.Emergency {
		t.Fatal("recovery failure must trigger emergency shutdown")
	}

	if !errors.Is(decision.Err, failure) {
		t.Fatalf("recovery failure lost: %v", decision.Err)
	}

	if decision.Reason !=
		"FLEX authorization recovery worker failure" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestRecoveryShutdownDecisionNilFailure(t *testing.T) {
	decision := recoveryFailureDecision(nil, true)

	if !decision.Emergency || decision.Err == nil {
		t.Fatal("nil recovery failure must fail closed")
	}
}

func TestRecoveryShutdownDecisionClosedChannel(t *testing.T) {
	recovery := make(chan error)
	close(recovery)

	decision := waitForSupervisedShutdownDecision(
		make(chan os.Signal),
		make(chan error),
		recovery,
		make(chan error),
	)

	if !decision.Emergency || decision.Err == nil {
		t.Fatal("closed recovery channel must fail closed")
	}
}

func TestRecoveryShutdownDecisionOverridesSignal(t *testing.T) {
	signals := make(chan os.Signal, 1)
	settlement := make(chan error)
	recovery := make(chan error, 1)
	server := make(chan error)

	signals <- syscall.SIGTERM

	failure := errors.New("recovery failure during shutdown")
	recovery <- failure

	decision := waitForSupervisedShutdownDecision(
		signals,
		settlement,
		recovery,
		server,
	)

	if !decision.Emergency {
		t.Fatal("pending recovery failure must override signal")
	}

	if !errors.Is(decision.Err, failure) {
		t.Fatalf("recovery failure lost: %v", decision.Err)
	}
}

func TestRecoveryShutdownDecisionPreservesLegacyPolicy(
	t *testing.T,
) {
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM

	decision := waitForSupervisedShutdownDecision(
		signals,
		make(chan error),
		nil,
		make(chan error),
	)

	if decision.Emergency || decision.Err != nil {
		t.Fatalf("legacy shutdown policy changed: %+v", decision)
	}
}

func TestRecoveryShutdownDecisionWaitsForFailure(
	t *testing.T,
) {
	signals := make(chan os.Signal)
	settlement := make(chan error)
	recovery := make(chan error, 1)
	server := make(chan error)

	result := make(chan shutdownDecision, 1)

	go func() {
		result <- waitForSupervisedShutdownDecision(
			signals,
			settlement,
			recovery,
			server,
		)
	}()

	select {
	case <-result:
		t.Fatal("shutdown selected without an event")
	case <-time.After(20 * time.Millisecond):
	}

	recovery <- errors.New("unexpected recovery exit")

	select {
	case decision := <-result:
		if !decision.Emergency {
			t.Fatal("recovery exit did not trigger emergency shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not respond to recovery failure")
	}
}
