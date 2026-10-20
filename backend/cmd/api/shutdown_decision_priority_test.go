package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestShutdownPriorityFlexFailureOverSignal(t *testing.T) {
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		signals := make(chan os.Signal, 1)
		flexFailures := make(chan error, 1)
		serverFailures := make(chan error, 1)

		failure := errors.New("simulated FLEX failure")

		signals <- syscall.SIGTERM
		flexFailures <- failure

		decision := waitForShutdownDecision(
			signals,
			flexFailures,
			serverFailures,
		)

		if !decision.Emergency {
			t.Fatalf(
				"iteration %d: FLEX failure lost emergency priority",
				i,
			)
		}

		if !errors.Is(decision.Err, failure) {
			t.Fatalf(
				"iteration %d: incorrect failure: %v",
				i,
				decision.Err,
			)
		}
	}
}

func TestShutdownPriorityHTTPFailureOverSignal(t *testing.T) {
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		signals := make(chan os.Signal, 1)
		flexFailures := make(chan error)
		serverFailures := make(chan error, 1)

		failure := errors.New("simulated HTTP failure")

		signals <- syscall.SIGTERM
		serverFailures <- failure

		decision := waitForShutdownDecision(
			signals,
			flexFailures,
			serverFailures,
		)

		if !decision.Emergency {
			t.Fatalf(
				"iteration %d: HTTP failure lost emergency priority",
				i,
			)
		}

		if !errors.Is(decision.Err, failure) {
			t.Fatalf(
				"iteration %d: incorrect failure: %v",
				i,
				decision.Err,
			)
		}
	}
}

func TestShutdownPriorityFlexOverHTTPFailure(t *testing.T) {
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		signals := make(chan os.Signal)
		flexFailures := make(chan error, 1)
		serverFailures := make(chan error, 1)

		flexErr := errors.New("FLEX failed")
		httpErr := errors.New("HTTP failed")

		flexFailures <- flexErr
		serverFailures <- httpErr

		decision := waitForShutdownDecision(
			signals,
			flexFailures,
			serverFailures,
		)

		if !errors.Is(decision.Err, flexErr) {
			t.Fatalf(
				"iteration %d: FLEX priority lost: %v",
				i,
				decision.Err,
			)
		}
	}
}

func TestShutdownPrioritySignalWithoutFailure(t *testing.T) {
	signals := make(chan os.Signal, 1)
	flexFailures := make(chan error)
	serverFailures := make(chan error)

	signals <- syscall.SIGTERM

	decision := waitForShutdownDecision(
		signals,
		flexFailures,
		serverFailures,
	)

	if decision.Emergency || decision.Err != nil {
		t.Fatalf(
			"normal SIGTERM incorrectly treated as failure: %+v",
			decision,
		)
	}
}
