package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// shutdownHTTPServer applies the production HTTP shutdown policy.
//
// Emergency shutdown closes active connections immediately.
// Normal shutdown allows active requests to finish before the
// deadline and forces closure if graceful draining fails.
func shutdownHTTPServer(
	server *http.Server,
	emergency bool,
	timeout time.Duration,
) error {
	if server == nil {
		return errors.New("HTTP server is nil")
	}

	if emergency {
		if err := server.Close(); err != nil {
			return fmt.Errorf(
				"emergency HTTP close: %w",
				err,
			)
		}

		return nil
	}

	if timeout <= 0 {
		return errors.New(
			"graceful HTTP shutdown timeout must be positive",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		timeout,
	)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		closeErr := server.Close()

		if closeErr != nil {
			return errors.Join(
				fmt.Errorf(
					"graceful HTTP shutdown: %w",
					err,
				),
				fmt.Errorf(
					"forced HTTP close: %w",
					closeErr,
				),
			)
		}

		return fmt.Errorf(
			"graceful HTTP shutdown timed out or failed: %w",
			err,
		)
	}

	return nil
}
