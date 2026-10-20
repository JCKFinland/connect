package main

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func startShutdownTestServer(
	t *testing.T,
	handler http.Handler,
) (*http.Server, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler: handler,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		_ = server.Close()
	})

	return server, "http://" + listener.Addr().String()
}

func TestHTTPShutdownPolicyGracefulDrain(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	server, address := startShutdownTestServer(
		t,
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("completed"))
		}),
	)

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	requestDone := make(chan error, 1)

	go func() {
		response, err := client.Get(address)
		if err != nil {
			requestDone <- err
			return
		}
		defer response.Body.Close()

		body, err := io.ReadAll(response.Body)
		if err == nil && string(body) != "completed" {
			err = io.ErrUnexpectedEOF
		}

		requestDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not enter handler")
	}

	shutdownDone := make(chan error, 1)

	go func() {
		shutdownDone <- shutdownHTTPServer(
			server,
			false,
			2*time.Second,
		)
	}()

	select {
	case err := <-shutdownDone:
		t.Fatalf(
			"graceful shutdown returned before request completed: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish")
	}

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("graceful shutdown failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("graceful shutdown did not finish")
	}
}

func TestHTTPShutdownPolicyEmergencyClose(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	server, address := startShutdownTestServer(
		t,
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusOK)
		}),
	)

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	requestDone := make(chan error, 1)

	go func() {
		response, err := client.Get(address)
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not enter handler")
	}

	if err := shutdownHTTPServer(
		server,
		true,
		time.Second,
	); err != nil {
		t.Fatalf("emergency shutdown failed: %v", err)
	}

	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal(
				"emergency shutdown unexpectedly completed active request",
			)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emergency shutdown did not interrupt request")
	}

	close(release)
}

func TestHTTPShutdownPolicyRejectsNilServer(t *testing.T) {
	if err := shutdownHTTPServer(
		nil,
		true,
		time.Second,
	); err == nil {
		t.Fatal("nil HTTP server accepted")
	}
}

func TestHTTPShutdownPolicyRejectsInvalidGracePeriod(
	t *testing.T,
) {
	server := &http.Server{}

	if err := shutdownHTTPServer(
		server,
		false,
		0,
	); err == nil {
		t.Fatal("zero graceful timeout accepted")
	}
}
