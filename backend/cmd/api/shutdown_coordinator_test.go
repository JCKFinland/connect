package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type coordinatorFlexWorker struct {
	mu sync.Mutex

	started chan struct{}
	release chan struct{}

	once sync.Once
}

func (w *coordinatorFlexWorker) StopAndWait(
	ctx context.Context,
) error {
	w.once.Do(func() {
		close(w.started)
	})

	select {
	case <-w.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestShutdownCoordinatorWaitsForAllWorkers(t *testing.T) {
	flex := &coordinatorFlexWorker{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})

	redispatchCancelled := make(chan struct{})
	presenceCancelled := make(chan struct{})

	result := make(chan error, 1)

	go func() {
		result <- coordinateShutdown(shutdownDependencies{
			HTTPServer:  &http.Server{},
			HTTPTimeout: time.Second,

			FlexWorker:  flex,
			FlexTimeout: time.Second,

			CancelRedispatch: func() {
				close(redispatchCancelled)
			},
			RedispatchDone: redispatchDone,

			CancelStalePresence: func() {
				close(presenceCancelled)
			},
			StalePresenceDone: presenceDone,
		})
	}()

	select {
	case <-flex.started:
	case <-time.After(time.Second):
		t.Fatal("FLEX shutdown was not reached")
	}

	select {
	case <-redispatchCancelled:
		t.Fatal("operational shutdown began before FLEX completed")
	default:
	}

	close(flex.release)

	select {
	case <-redispatchCancelled:
	case <-time.After(time.Second):
		t.Fatal("redispatch cancellation not issued")
	}

	select {
	case <-presenceCancelled:
	case <-time.After(time.Second):
		t.Fatal("presence cancellation not issued")
	}

	select {
	case err := <-result:
		t.Fatalf("coordinator returned before worker completion: %v", err)
	default:
	}

	close(redispatchDone)

	select {
	case err := <-result:
		t.Fatalf("coordinator returned before presence completion: %v", err)
	default:
	}

	close(presenceDone)

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("coordinator did not finish")
	}
}

func TestShutdownCoordinatorRejectsMissingDependencies(t *testing.T) {
	err := coordinateShutdown(shutdownDependencies{})

	if err == nil {
		t.Fatal("incomplete shutdown dependencies accepted")
	}
}

func TestShutdownCoordinatorPropagatesFlexError(t *testing.T) {
	flex := newControlledFlexShutdownWaiter()
	expected := errors.New("financial worker failure")
	flex.completionErr = expected
	close(flex.done)

	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})
	close(redispatchDone)
	close(presenceDone)

	err := coordinateShutdown(shutdownDependencies{
		HTTPServer:  &http.Server{},
		HTTPTimeout: time.Second,

		FlexWorker:  flex,
		FlexTimeout: time.Second,

		CancelRedispatch: func() {},
		RedispatchDone:   redispatchDone,

		CancelStalePresence: func() {},
		StalePresenceDone:   presenceDone,
	})

	if !errors.Is(err, expected) {
		t.Fatalf("expected financial error, got %v", err)
	}
}
