package dispatch

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestRedispatchWorkerDoesNotStartWithoutDatabase(t *testing.T) {
	var output bytes.Buffer

	log := slog.New(
		slog.NewTextHandler(
			&output,
			&slog.HandlerOptions{
				Level: slog.LevelDebug,
			},
		),
	)

	service := &Service{
		log: log,
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	defer cancel()

	service.StartRedispatchWorker(
		ctx,
		RedispatchWorkerOptions{
			Interval:  10 * time.Millisecond,
			BatchSize: 10,
		},
	)

	// A worker without a database cannot safely run.
	// It must return without panicking or emitting misleading
	// "worker started" lifecycle logs.
	if output.Len() != 0 {
		t.Fatalf(
			"expected no worker logs without database, got %q",
			output.String(),
		)
	}
}

func TestRedispatchWorkerLoggerFallback(t *testing.T) {
	service := NewService(
		Dependencies{},
	)

	if service == nil {
		t.Fatal(
			"expected dispatch service",
		)
	}

	if service.log == nil {
		t.Fatal(
			"expected fallback logger to be configured",
		)
	}
}
