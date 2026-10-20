package main

import (
	"sync"
	"testing"
	"time"
)

func TestBackgroundShutdownWaitsForBothWorkers(t *testing.T) {
	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})

	redispatchCancelled := make(chan struct{})
	presenceCancelled := make(chan struct{})

	var onceRedispatch sync.Once
	var oncePresence sync.Once

	cancelRedispatch := func() {
		onceRedispatch.Do(func() {
			close(redispatchCancelled)
		})
	}

	cancelPresence := func() {
		oncePresence.Do(func() {
			close(presenceCancelled)
		})
	}

	shutdownDone := make(chan struct{})

	go func() {
		stopAndWaitBackgroundWorkers(
			cancelRedispatch,
			redispatchDone,
			cancelPresence,
			presenceDone,
		)
		close(shutdownDone)
	}()

	select {
	case <-redispatchCancelled:
	case <-time.After(time.Second):
		t.Fatal("redispatch cancellation not requested")
	}

	select {
	case <-presenceCancelled:
	case <-time.After(time.Second):
		t.Fatal("presence cancellation not requested")
	}

	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned before worker completion")
	default:
	}

	close(redispatchDone)

	select {
	case <-shutdownDone:
		t.Fatal(
			"shutdown returned while stale-presence worker remained active",
		)
	default:
	}

	close(presenceDone)

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after both workers exited")
	}
}

func TestBackgroundShutdownAlreadyCompletedWorkers(t *testing.T) {
	redispatchDone := make(chan struct{})
	presenceDone := make(chan struct{})

	close(redispatchDone)
	close(presenceDone)

	redispatchCancelled := false
	presenceCancelled := false

	stopAndWaitBackgroundWorkers(
		func() {
			redispatchCancelled = true
		},
		redispatchDone,
		func() {
			presenceCancelled = true
		},
		presenceDone,
	)

	if !redispatchCancelled || !presenceCancelled {
		t.Fatal("both workers must receive cancellation")
	}
}
