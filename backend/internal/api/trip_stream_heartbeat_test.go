package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/realtime"
)

// heartbeatObservedWriter reports completed SSE flushes.
// The response body is read only by the streaming goroutine
// or after that goroutine has terminated.
type heartbeatObservedWriter struct {
	gin.ResponseWriter

	recorder *httptest.ResponseRecorder

	connectedOnce sync.Once
	heartbeatOnce sync.Once

	connected chan struct{}
	heartbeat chan struct{}
}

func (w *heartbeatObservedWriter) Flush() {
	w.ResponseWriter.Flush()

	body := w.recorder.Body.String()

	if strings.Contains(body, "event:connected") {
		w.connectedOnce.Do(func() {
			close(w.connected)
		})
	}

	if strings.Contains(body, "event:heartbeat") {
		w.heartbeatOnce.Do(func() {
			close(w.heartbeat)
		})
	}
}

func TestTripStreamHeartbeatAuthorization(t *testing.T) {
	tests := []struct {
		name          string
		allowed       bool
		membershipErr error
		wantHeartbeat bool
	}{
		{
			name:          "authorized membership",
			allowed:       true,
			wantHeartbeat: true,
		},
		{
			name:          "revoked membership",
			allowed:       false,
			wantHeartbeat: false,
		},
		{
			name:          "membership lookup failure",
			allowed:       true,
			membershipErr: errors.New("membership backend unavailable"),
			wantHeartbeat: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			memberships := &streamRevocationMembershipRepository{
				allowed: true,
			}

			heartbeatEvents := make(chan time.Time)

			handler := NewTripStreamHandler(
				newRevocationStreamService(memberships),
				realtime.NewBroker(),
			)
			handler.heartbeat = heartbeatEvents

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c, recorder := newTripStreamTestContext(
				t,
				ctx,
				"dispatcher",
			)

			observer := &heartbeatObservedWriter{
				ResponseWriter: c.Writer,
				recorder:       recorder,
				connected:      make(chan struct{}),
				heartbeat:      make(chan struct{}),
			}

			c.Writer = observer

			done := make(chan struct{})

			go func() {
				defer close(done)
				handler.Stream(c)
			}()

			// Wait until initial authorization has succeeded
			// and the connected SSE event has been flushed.
			select {
			case <-observer.connected:
			case <-done:
				t.Fatal("stream exited before connection")
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("stream did not establish connection")
			}

			// Apply the membership state only after connection.
			memberships.set(tc.allowed, tc.membershipErr)

			select {
			case heartbeatEvents <- time.Now():
			case <-done:
				t.Fatal("stream exited before heartbeat delivery")
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("stream did not receive heartbeat")
			}

			if tc.wantHeartbeat {
				// Wait for the actual heartbeat flush before
				// canceling the authorized connection.
				select {
				case <-observer.heartbeat:
				case <-done:
					t.Fatal("authorized stream exited before heartbeat")
				case <-time.After(2 * time.Second):
					cancel()
					<-done
					t.Fatal("authorized heartbeat was not flushed")
				}

				cancel()
			}

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cancel()
				<-done
				t.Fatal("stream did not terminate")
			}

			body := recorder.Body.String()

			if !strings.Contains(body, "event:connected") {
				t.Fatalf("missing connected event: %q", body)
			}

			got := strings.Count(body, "event:heartbeat")

			want := 0
			if tc.wantHeartbeat {
				want = 1
			}

			if got != want {
				t.Fatalf(
					"expected %d heartbeat events, got %d: %q",
					want,
					got,
					body,
				)
			}
		})
	}
}
