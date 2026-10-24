package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// shutdownDependencies holds the resources whose shutdown
// must complete before main returns and closes PostgreSQL.
type shutdownDependencies struct {
	HTTPServer  *http.Server
	Emergency   bool
	HTTPTimeout time.Duration

	FlexWorker  flexShutdownWaiter
	FlexTimeout time.Duration

	// RecoveryWorker is independently supervised and must
	// complete before PostgreSQL resources are released.
	RecoveryWorker  flexShutdownWaiter
	RecoveryTimeout time.Duration

	CancelRedispatch    func()
	RedispatchDone      <-chan struct{}
	CancelStalePresence func()
	StalePresenceDone   <-chan struct{}
}

// coordinateShutdown preserves the production resource
// ordering and never treats a worker cancellation request
// as proof that its goroutine has terminated.
func coordinateShutdown(
	deps shutdownDependencies,
) error {
	if deps.HTTPServer == nil {
		return errors.New("shutdown HTTP server is nil")
	}

	if deps.CancelRedispatch == nil ||
		deps.RedispatchDone == nil ||
		deps.CancelStalePresence == nil ||
		deps.StalePresenceDone == nil {
		return errors.New(
			"operational shutdown dependencies are incomplete",
		)
	}

	var shutdownErrors []error

	if err := shutdownHTTPServer(
		deps.HTTPServer,
		deps.Emergency,
		deps.HTTPTimeout,
	); err != nil {
		shutdownErrors = append(
			shutdownErrors,
			fmt.Errorf("HTTP shutdown: %w", err),
		)
	}

	if deps.FlexWorker != nil {
		if err := waitForFlexShutdown(
			deps.FlexWorker,
			deps.FlexTimeout,
		); err != nil {
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf("FLEX shutdown: %w", err),
			)
		}
	}

	if deps.RecoveryWorker != nil {
		if err := waitForFlexShutdown(
			deps.RecoveryWorker,
			deps.RecoveryTimeout,
		); err != nil {
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf("FLEX authorization recovery shutdown: %w", err),
			)
		}
	}

	stopAndWaitBackgroundWorkers(
		deps.CancelRedispatch,
		deps.RedispatchDone,
		deps.CancelStalePresence,
		deps.StalePresenceDone,
	)

	return errors.Join(shutdownErrors...)
}
