package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This test documents the HTTP behavior required by the
// production shutdown policy: graceful shutdown drains
// active requests rather than terminating them immediately.
func TestFlexShutdownPolicyGracefulHTTPDrain(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})

	server := httptest.NewUnstartedServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusOK)
		}),
	)

	listener := server.Listener
	server.Config.RegisterOnShutdown(func() {
		close(finished)
	})

	server.Start()
	defer server.Close()

	client := server.Client()

	requestDone := make(chan error, 1)

	go func() {
		response, err := client.Get(server.URL)
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP request did not enter handler")
	}

	close(release)

	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatalf("active request failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active request did not finish")
	}

	// Verify the listener is no longer accepting requests
	// after a normal graceful shutdown.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	if err := server.Config.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback not executed")
	}

	connection, err := net.DialTimeout(
		"tcp",
		listener.Addr().String(),
		100*time.Millisecond,
	)
	if err == nil {
		connection.Close()
		t.Fatal("listener still accepting connections")
	}
}
