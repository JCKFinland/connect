package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/realtime"
)

func TestTripStreamHeartbeatStopsAfterMembershipRevocation(t *testing.T) {
	memberships := &streamRevocationMembershipRepository{
		allowed: true,
	}

	heartbeat := make(chan time.Time)

	handler := NewTripStreamHandler(
		newRevocationStreamService(memberships),
		realtime.NewBroker(),
	)
	handler.heartbeat = heartbeat

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, recorder := newTripStreamTestContext(
		t,
		ctx,
		"dispatcher",
	)

	done := make(chan struct{})

	go func() {
		defer close(done)
		handler.Stream(c)
	}()

	// The first heartbeat proves that the stream is active.
	select {
	case heartbeat <- time.Now():
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("stream did not become ready")
	}

	// Revoke membership before triggering the next heartbeat.
	memberships.set(false, nil)

	select {
	case heartbeat <- time.Now():
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("stream did not receive revoked heartbeat")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("stream did not terminate after revocation")
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "event:connected") {
		t.Fatalf("missing connected event: %q", body)
	}

	if strings.Count(body, "event:heartbeat") > 1 {
		t.Fatalf("heartbeat emitted after revocation: %q", body)
	}
}
