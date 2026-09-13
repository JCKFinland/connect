package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/realtime"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type tripStreamTestTripRepository struct {
	repository.TripRepository

	result *models.Trip
	err    error
}

func (r *tripStreamTestTripRepository) GetByID(
	_ context.Context,
	_ string,
) (*models.Trip, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.result, nil
}

type tripStreamTestRoleRepository struct {
	repository.UserRoleRepository

	roles []string
	err   error
}

func (r *tripStreamTestRoleRepository) GetUserRoles(
	_ context.Context,
	_ string,
) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.roles, nil
}

func newTripStreamTestService(
	tripResult *models.Trip,
	tripErr error,
	roles []string,
) trip.Service {
	return trip.NewService(
		trip.Dependencies{
			Trips: &tripStreamTestTripRepository{
				result: tripResult,
				err:    tripErr,
			},
			UserRoles: &tripStreamTestRoleRepository{
				roles: roles,
			},
		},
	)
}

func newTripStreamTestContext(
	t *testing.T,
	ctx context.Context,
	userID string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/trip-1/stream",
		nil,
	)

	if ctx != nil {
		request = request.WithContext(ctx)
	}

	c.Request = request

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "trip-1",
		},
	}

	if userID != "" {
		user := &models.User{}
		user.ID = userID

		middleware.SetCurrentUser(
			c,
			user,
		)
	}

	return c, recorder
}

func TestTripStreamReturnsUnauthorizedWithoutCurrentUser(
	t *testing.T,
) {
	service := newTripStreamTestService(
		nil,
		nil,
		nil,
	)

	handler := NewTripStreamHandler(
		service,
		realtime.NewBroker(),
	)

	c, recorder := newTripStreamTestContext(
		t,
		context.Background(),
		"",
	)

	handler.Stream(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestTripStreamReturnsForbiddenForUnrelatedUser(
	t *testing.T,
) {
	const unrelatedUserID = "customer-other"

	service := newTripStreamTestService(
		&models.Trip{
			BaseModel: models.BaseModel{
				ID: "trip-1",
			},
			CustomerID: "customer-owner",
			DriverID:   "driver-owner",
		},
		nil,
		[]string{"CUSTOMER"},
	)

	handler := NewTripStreamHandler(
		service,
		realtime.NewBroker(),
	)

	c, recorder := newTripStreamTestContext(
		t,
		context.Background(),
		unrelatedUserID,
	)

	handler.Stream(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusForbidden,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestTripStreamReturnsNotFoundForMissingTrip(
	t *testing.T,
) {
	service := newTripStreamTestService(
		nil,
		repository.ErrNotFound,
		[]string{"CUSTOMER"},
	)

	handler := NewTripStreamHandler(
		service,
		realtime.NewBroker(),
	)

	c, recorder := newTripStreamTestContext(
		t,
		context.Background(),
		"customer-1",
	)

	handler.Stream(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusNotFound,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestTripStreamEmitsConnectedEvent(
	t *testing.T,
) {
	const customerID = "customer-1"

	service := newTripStreamTestService(
		&models.Trip{
			BaseModel: models.BaseModel{
				ID: "trip-1",
			},
			CustomerID: customerID,
		},
		nil,
		[]string{"CUSTOMER"},
	)

	broker := realtime.NewBroker()

	handler := NewTripStreamHandler(
		service,
		broker,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	c, recorder := newTripStreamTestContext(
		t,
		ctx,
		customerID,
	)

	done := make(chan struct{})

	go func() {
		defer close(done)
		handler.Stream(c)
	}()

	// Give the handler enough time to authorize, subscribe,
	// and write its initial connected event before cancellation.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-done:

	case <-time.After(time.Second):
		t.Fatal(
			"stream did not terminate after request cancellation",
		)
	}

	// ResponseRecorder is not concurrency-safe. Read its body only
	// after the streaming goroutine has completely stopped.
	body := recorder.Body.String()

	if !strings.Contains(
		body,
		"event:connected",
	) {
		t.Fatalf(
			"connected event was not emitted: %q",
			body,
		)
	}

	if !strings.Contains(
		body,
		`"trip_id":"trip-1"`,
	) {
		t.Fatalf(
			"connected event missing trip ID: %q",
			body,
		)
	}
}

func TestTripStreamForwardsBrokerEvent(
	t *testing.T,
) {
	const customerID = "customer-1"

	service := newTripStreamTestService(
		&models.Trip{
			BaseModel: models.BaseModel{
				ID: "trip-1",
			},
			CustomerID: customerID,
		},
		nil,
		[]string{"CUSTOMER"},
	)

	broker := realtime.NewBroker()

	handler := NewTripStreamHandler(
		service,
		broker,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	defer cancel()

	c, recorder := newTripStreamTestContext(
		t,
		ctx,
		customerID,
	)

	done := make(chan struct{})

	go func() {
		defer close(done)
		handler.Stream(c)
	}()

	// Allow the stream handler to complete authorization and establish
	// its broker subscription before publishing the test event.
	time.Sleep(50 * time.Millisecond)

	broker.Publish(
		"trip:trip-1",
		realtime.Event{
			Type: "trip.location",
			Data: gin.H{
				"latitude":  60.1699,
				"longitude": 24.9384,
			},
		},
	)

	// Allow the subscribed stream goroutine to consume and render
	// the broker event before cancellation.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-done:

	case <-time.After(time.Second):
		t.Fatal(
			"stream did not terminate after cancellation",
		)
	}

	// ResponseRecorder is not concurrency-safe. Inspect it only after
	// the stream goroutine has terminated.
	body := recorder.Body.String()

	if !strings.Contains(
		body,
		"event:connected",
	) {
		t.Fatalf(
			"connected event was not emitted: %q",
			body,
		)
	}

	if !strings.Contains(
		body,
		"event:trip.location",
	) {
		t.Fatalf(
			"broker event was not streamed: %q",
			body,
		)
	}

	if !strings.Contains(
		body,
		`"latitude":60.1699`,
	) {
		t.Fatalf(
			"streamed event missing latitude: %q",
			body,
		)
	}

	if !strings.Contains(
		body,
		`"longitude":24.9384`,
	) {
		t.Fatalf(
			"streamed event missing longitude: %q",
			body,
		)
	}
}
