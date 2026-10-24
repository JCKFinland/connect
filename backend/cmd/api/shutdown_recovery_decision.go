package main

import (
	"errors"
	"os"
)

// recoveryFailureDecision reports unexpected authorization
// recovery termination as an emergency application failure.
func recoveryFailureDecision(
	err error,
	ok bool,
) shutdownDecision {
	if !ok {
		err = errors.New(
			"FLEX authorization recovery failure channel closed",
		)
	} else if err == nil {
		err = errors.New(
			"FLEX authorization recovery worker exited unexpectedly",
		)
	}

	return shutdownDecision{
		Reason:    "FLEX authorization recovery worker failure",
		Emergency: true,
		Err:       err,
	}
}

// waitForSupervisedShutdownDecision extends the existing
// shutdown policy with an independent recovery failure source.
//
// Pending financial failures take precedence over ordinary
// operating-system shutdown signals.
//
// Existing waitForShutdownDecision callers remain unchanged.
func waitForSupervisedShutdownDecision(
	signals <-chan os.Signal,
	settlementFailures <-chan error,
	recoveryFailures <-chan error,
	serverFailures <-chan error,
) shutdownDecision {
	// Preserve existing behavior when recovery supervision
	// is not configured.
	if recoveryFailures == nil {
		return waitForShutdownDecision(
			signals,
			settlementFailures,
			serverFailures,
		)
	}

	var decision shutdownDecision

	select {
	case _, ok := <-signals:
		if !ok {
			decision = shutdownDecision{
				Reason:    "operating system signal channel closed",
				Emergency: true,
				Err: errors.New(
					"shutdown signal channel closed",
				),
			}
		} else {
			decision = shutdownDecision{
				Reason: "operating system signal",
			}
		}

	case err, ok := <-settlementFailures:
		decision = flexFailureDecision(err, ok)

	case err, ok := <-recoveryFailures:
		decision = recoveryFailureDecision(err, ok)

	case err, ok := <-serverFailures:
		decision = serverFailureDecision(err, ok)
	}

	// Settlement failure takes first precedence.
	select {
	case err, ok := <-settlementFailures:
		return flexFailureDecision(err, ok)
	default:
	}

	// Recovery failure also overrides ordinary shutdown.
	select {
	case err, ok := <-recoveryFailures:
		return recoveryFailureDecision(err, ok)
	default:
	}

	// HTTP listener failure overrides normal shutdown.
	select {
	case err, ok := <-serverFailures:
		if !decision.Emergency {
			return serverFailureDecision(err, ok)
		}
	default:
	}

	return decision
}
