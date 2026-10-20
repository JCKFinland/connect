package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// flexShutdownWaiter is the minimum financial lifecycle
// contract required to protect database cleanup.
type flexShutdownWaiter interface {
	StopAndWait(context.Context) error
}

// waitForFlexShutdown guarantees that a timeout cannot
// be mistaken for proof of worker completion.
//
// An expired initial deadline triggers an unbounded
// completion wait to preserve the database safety barrier.
//
// A non-context worker error is returned immediately:
// the production lifecycle reports it only after the
// worker completion channel has closed.
func waitForFlexShutdown(
	worker flexShutdownWaiter,
	timeout time.Duration,
) error {
	if worker == nil {
		return errors.New("FLEX shutdown worker is nil")
	}

	if timeout <= 0 {
		return errors.New(
			"FLEX shutdown timeout must be positive",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		timeout,
	)

	err := worker.StopAndWait(ctx)
	cancel()

	if err == nil {
		return nil
	}

	if !errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, context.Canceled) {
		return err
	}

	// Never permit PostgreSQL cleanup merely because
	// the bounded shutdown deadline has expired.
	if completionErr := worker.StopAndWait(
		context.Background(),
	); completionErr != nil {
		return fmt.Errorf(
			"FLEX worker completion after shutdown deadline: %w",
			completionErr,
		)
	}

	return nil
}
