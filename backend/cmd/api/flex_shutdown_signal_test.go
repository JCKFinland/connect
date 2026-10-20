package main

import (
	"errors"
	"testing"
)

// This test verifies that a buffered failure notification
// cannot block worker completion.
func TestFlexFailureNotificationNonblocking(t *testing.T) {
	failures := make(chan error, 1)

	first := errors.New("first failure")
	second := errors.New("second failure")

	notify := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}

	notify(first)
	notify(second)

	select {
	case received := <-failures:
		if !errors.Is(received, first) {
			t.Fatalf(
				"unexpected failure notification: %v",
				received,
			)
		}
	default:
		t.Fatal("missing failure notification")
	}
}
