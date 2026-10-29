package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/realtime"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type streamRevocationMembershipRepository struct {
	repository.CompanyMembershipRepository

	mu      sync.RWMutex
	allowed bool
	err     error
}

func (r *streamRevocationMembershipRepository) Exists(
	_ context.Context,
	_, _ string,
) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.allowed, r.err
}

func (r *streamRevocationMembershipRepository) set(
	allowed bool,
	err error,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.allowed = allowed
	r.err = err
}

func newRevocationStreamService(
	memberships *streamRevocationMembershipRepository,
) trip.Service {
	return trip.NewService(trip.Dependencies{
		Trips: &tripStreamTestTripRepository{
			result: &models.Trip{
				BaseModel: models.BaseModel{
					ID: "trip-1",
				},
				CompanyID:  "company-a",
				CustomerID: "customer-owner",
				DriverID:   "driver-owner",
			},
		},
		UserRoles: &tripStreamTestRoleRepository{
			roles: []string{"DISPATCHER"},
		},
		CompanyMemberships: memberships,
	})
}

func TestTripStreamStopsAfterMembershipRevocation(
	t *testing.T,
) {
	testTripStreamAuthorizationChange(
		t,
		false,
		nil,
	)
}

func TestTripStreamStopsAfterMembershipLookupFailure(
	t *testing.T,
) {
	testTripStreamAuthorizationChange(
		t,
		true,
		errors.New("membership backend unavailable"),
	)
}

func testTripStreamAuthorizationChange(
	t *testing.T,
	allowedAfterChange bool,
	errAfterChange error,
) {
	t.Helper()

	memberships := &streamRevocationMembershipRepository{
		allowed: true,
	}

	broker := realtime.NewBroker()

	handler := NewTripStreamHandler(
		newRevocationStreamService(memberships),
		broker,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
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

	// Wait for initial authorization and subscription.
	// Existing test infrastructure uses the same startup interval.
	time.Sleep(100 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("stream terminated before authorization changed")
	default:
	}

	memberships.set(
		allowedAfterChange,
		errAfterChange,
	)

	broker.Publish(
		"trip:trip-1",
		realtime.Event{
			Type: "trip.location",
			Data: map[string]any{
				"latitude": 60.1699,
			},
		},
	)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not terminate after authorization change")
	}

	// ResponseRecorder is inspected only after the handler exits.
	body := recorder.Body.String()

	if !strings.Contains(body, "event:connected") {
		t.Fatalf("missing initial connected event: %q", body)
	}

	if strings.Contains(body, "event:trip.location") {
		t.Fatalf(
			"revoked stream forwarded sensitive event: %q",
			body,
		)
	}
}

func TestTripStreamContinuesWhileMembershipValid(
	t *testing.T,
) {
	memberships := &streamRevocationMembershipRepository{
		allowed: true,
	}

	broker := realtime.NewBroker()

	handler := NewTripStreamHandler(
		newRevocationStreamService(memberships),
		broker,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
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

	time.Sleep(100 * time.Millisecond)

	broker.Publish(
		"trip:trip-1",
		realtime.Event{
			Type: "trip.location",
			Data: map[string]any{
				"latitude": 60.1699,
			},
		},
	)

	time.Sleep(100 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("authorized stream terminated unexpectedly")
	default:
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("authorized stream did not stop after cancellation")
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d",
			recorder.Code,
		)
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "event:trip.location") {
		t.Fatalf(
			"authorized stream did not forward event: %q",
			body,
		)
	}
}
