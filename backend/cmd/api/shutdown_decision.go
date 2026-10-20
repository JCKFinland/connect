package main

import (
	"errors"
	"os"
)

// shutdownDecision captures the reason and HTTP shutdown
// policy selected by the production shutdown coordinator.
type shutdownDecision struct {
	Reason    string
	Emergency bool
	Err       error
}

func waitForShutdownDecision(
	signals <-chan os.Signal,
	flexFailures <-chan error,
	serverFailures <-chan error,
) shutdownDecision {
	// Wait for the first shutdown event.
	// The result is provisional until pending internal
	// failures have been inspected.
	var decision shutdownDecision

	select {
	case _, ok := <-signals:
		if !ok {
			decision = shutdownDecision{
				Reason:    "operating system signal channel closed",
				Emergency: true,
				Err:       errors.New("shutdown signal channel closed"),
			}
		} else {
			decision = shutdownDecision{
				Reason: "operating system signal",
			}
		}

	case err, ok := <-flexFailures:
		decision = flexFailureDecision(err, ok)

	case err, ok := <-serverFailures:
		decision = serverFailureDecision(err, ok)
	}

	// A pending financial worker failure always overrides
	// a normal operating-system shutdown decision.
	//
	// Nonblocking checks ensure that no additional event
	// is required before shutdown proceeds.
	select {
	case err, ok := <-flexFailures:
		return flexFailureDecision(err, ok)
	default:
	}

	// An HTTP listener failure also overrides a normal
	// operating-system shutdown decision.
	select {
	case err, ok := <-serverFailures:
		if !decision.Emergency {
			return serverFailureDecision(err, ok)
		}
	default:
	}

	return decision
}

func flexFailureDecision(
	err error,
	ok bool,
) shutdownDecision {
	if !ok {
		err = errors.New("FLEX failure channel closed")
	} else if err == nil {
		err = errors.New("FLEX worker exited unexpectedly")
	}

	return shutdownDecision{
		Reason:    "FLEX settlement worker failure",
		Emergency: true,
		Err:       err,
	}
}

func serverFailureDecision(
	err error,
	ok bool,
) shutdownDecision {
	if !ok {
		err = errors.New("HTTP listener failure channel closed")
	} else if err == nil {
		err = errors.New("HTTP listener exited unexpectedly")
	}

	return shutdownDecision{
		Reason:    "HTTP listener failure",
		Emergency: true,
		Err:       err,
	}
}
