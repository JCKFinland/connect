package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

type integrationFlexWorker struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	err     error
}

func (w *integrationFlexWorker) StopAndWait(
	ctx context.Context,
) error {
	w.once.Do(func() {
		close(w.started)
	})

	select {
	case <-w.release:
		return w.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newIntegrationHTTPServer(
	t *testing.T,
) (*http.Server, <-chan struct{}) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	stopped := make(chan struct{})

	server := &http.Server{
		Handler: http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	go func() {
		defer close(stopped)
		_ = server.Serve(listener)
	}()

	return server, stopped
}

func TestShutdownIntegrationWaitsForFinancialCompletion(
	t *testing.T,
) {
	server, httpStopped := newIntegrationHTTPServer(t)

	flex := &integrationFlexWorker{
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
			HTTPServer:  server,
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
	case <-httpStopped:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP server did not stop")
	}

	select {
	case <-flex.started:
	case <-time.After(3 * time.Second):
		t.Fatal("financial shutdown did not begin")
	}

	select {
	case <-redispatchCancelled:
		t.Fatal("operational cleanup began before financial completion")
	default:
	}

	select {
	case <-result:
		t.Fatal("shutdown returned before financial completion")
	default:
	}

	close(flex.release)

	select {
	case <-redispatchCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("redispatch cancellation missing")
	}

	select {
	case <-presenceCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("presence cancellation missing")
	}

	select {
	case <-result:
		t.Fatal("shutdown returned before operational completion")
	default:
	}

	close(redispatchDone)
	close(presenceDone)

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown coordinator did not finish")
	}
}

func TestShutdownIntegrationPropagatesFinancialFailure(
	t *testing.T,
) {
	server, _ := newIntegrationHTTPServer(t)

	expected := errors.New("financial worker failed")

	flex := &integrationFlexWorker{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     expected,
	}

	close(flex.release)

	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})

	close(redispatchDone)
	close(presenceDone)

	err := coordinateShutdown(shutdownDependencies{
		HTTPServer:          server,
		HTTPTimeout:         time.Second,
		FlexWorker:          flex,
		FlexTimeout:         time.Second,
		CancelRedispatch:    func() {},
		RedispatchDone:      redispatchDone,
		CancelStalePresence: func() {},
		StalePresenceDone:   presenceDone,
	})

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected financial failure, got %v",
			err,
		)
	}
}

func TestShutdownIntegrationDisabledFlexWorker(
	t *testing.T,
) {
	server, _ := newIntegrationHTTPServer(t)

	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})

	close(redispatchDone)
	close(presenceDone)

	err := coordinateShutdown(shutdownDependencies{
		HTTPServer:          server,
		HTTPTimeout:         time.Second,
		FlexWorker:          nil,
		FlexTimeout:         time.Second,
		CancelRedispatch:    func() {},
		RedispatchDone:      redispatchDone,
		CancelStalePresence: func() {},
		StalePresenceDone:   presenceDone,
	})

	if err != nil {
		t.Fatalf(
			"disabled FLEX shutdown must succeed: %v",
			err,
		)
	}
}
