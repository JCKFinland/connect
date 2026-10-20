package main

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestShutdownDecisionOperatingSystemSignal(t *testing.T) {
	signals := make(chan os.Signal, 1)
	flexFailures := make(chan error)
	serverFailures := make(chan error)

	signals <- syscall.SIGTERM

	decision := waitForShutdownDecision(
		signals,
		flexFailures,
		serverFailures,
	)

	if decision.Emergency {
		t.Fatal("ordinary SIGTERM must not force emergency shutdown")
	}

	if decision.Err != nil {
		t.Fatalf("unexpected shutdown error: %v", decision.Err)
	}

	if decision.Reason != "operating system signal" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestShutdownDecisionFlexWorkerFailure(t *testing.T) {
	signals := make(chan os.Signal)
	flexFailures := make(chan error, 1)
	serverFailures := make(chan error)

	failure := errors.New("simulated settlement worker failure")
	flexFailures <- failure

	decision := waitForShutdownDecision(
		signals,
		flexFailures,
		serverFailures,
	)

	if !decision.Emergency {
		t.Fatal("FLEX failure must trigger emergency shutdown")
	}

	if !errors.Is(decision.Err, failure) {
		t.Fatalf("unexpected failure: %v", decision.Err)
	}

	if decision.Reason != "FLEX settlement worker failure" {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestShutdownDecisionHTTPListenerFailure(t *testing.T) {
	signals := make(chan os.Signal)
	flexFailures := make(chan error)
	serverFailures := make(chan error, 1)

	failure := errors.New("simulated listener failure")
	serverFailures <- failure

	decision := waitForShutdownDecision(
		signals,
		flexFailures,
		serverFailures,
	)

	if !decision.Emergency {
		t.Fatal("HTTP listener failure must trigger emergency shutdown")
	}

	if !errors.Is(decision.Err, failure) {
		t.Fatalf("unexpected failure: %v", decision.Err)
	}
}

func TestShutdownDecisionNilWorkerFailure(t *testing.T) {
	signals := make(chan os.Signal)
	flexFailures := make(chan error, 1)
	serverFailures := make(chan error)

	flexFailures <- nil

	decision := waitForShutdownDecision(
		signals,
		flexFailures,
		serverFailures,
	)

	if !decision.Emergency || decision.Err == nil {
		t.Fatal("unexpected nil worker exit must fail closed")
	}
}

func TestShutdownDecisionClosedFailureChannel(t *testing.T) {
	signals := make(chan os.Signal)
	flexFailures := make(chan error)
	serverFailures := make(chan error)

	close(flexFailures)

	decision := waitForShutdownDecision(
		signals,
		flexFailures,
		serverFailures,
	)

	if !decision.Emergency || decision.Err == nil {
		t.Fatal("closed failure channel must fail closed")
	}
}

func TestShutdownDecisionWaitsForEvent(t *testing.T) {
	signals := make(chan os.Signal)
	flexFailures := make(chan error, 1)
	serverFailures := make(chan error)

	result := make(chan shutdownDecision, 1)

	go func() {
		result <- waitForShutdownDecision(
			signals,
			flexFailures,
			serverFailures,
		)
	}()

	select {
	case <-result:
		t.Fatal("shutdown selected without a trigger")
	case <-time.After(20 * time.Millisecond):
	}

	flexFailures <- errors.New("worker terminated")

	select {
	case decision := <-result:
		if !decision.Emergency {
			t.Fatal("worker termination did not trigger emergency shutdown")
		}
		if !strings.Contains(decision.Reason, "FLEX") {
			t.Fatalf("unexpected reason: %q", decision.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown decision blocked after worker failure")
	}
}
