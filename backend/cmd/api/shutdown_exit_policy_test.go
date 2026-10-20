package main

import (
	"errors"
	"testing"
)

func TestShutdownExitCodeNormalSignal(t *testing.T) {
	decision := shutdownDecision{
		Reason: "operating system signal",
	}

	if got := shutdownExitCode(decision, nil); got != 0 {
		t.Fatalf("expected exit code 0, got %d", got)
	}
}

func TestShutdownExitCodeFlexFailure(t *testing.T) {
	decision := shutdownDecision{
		Reason:    "FLEX settlement worker failure",
		Emergency: true,
		Err:       errors.New("worker failure"),
	}

	if got := shutdownExitCode(decision, nil); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}

func TestShutdownExitCodeHTTPFailure(t *testing.T) {
	decision := shutdownDecision{
		Reason:    "HTTP listener failure",
		Emergency: true,
		Err:       errors.New("listener failure"),
	}

	if got := shutdownExitCode(decision, nil); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}

func TestShutdownExitCodeCleanupFailure(t *testing.T) {
	decision := shutdownDecision{
		Reason: "operating system signal",
	}

	if got := shutdownExitCode(
		decision,
		errors.New("shutdown failed"),
	); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}

func TestShutdownExitCodeEmergencyWithoutError(t *testing.T) {
	decision := shutdownDecision{
		Emergency: true,
	}

	if got := shutdownExitCode(decision, nil); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}
