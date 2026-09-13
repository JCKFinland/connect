package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/realtime"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type tripLocationRealtimeTestService struct {
	trip.Service

	location *models.TripLocation
	err      error

	called      bool
	tripID      string
	actorUserID string
	request     trip.RecordLocationRequest
}

func (s *tripLocationRealtimeTestService) RecordTripLocation(
	_ context.Context,
	tripID string,
	actorUserID string,
	req trip.RecordLocationRequest,
) (*models.TripLocation, error) {
	s.called = true
	s.tripID = tripID
	s.actorUserID = actorUserID
	s.request = req

	if s.err != nil {
		return nil, s.err
	}

	return s.location, nil
}

func newTripLocationRealtimeTestContext(
	t *testing.T,
	tripID string,
	userID string,
	recordedAt time.Time,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	body := strings.NewReader(
		`{` +
			`"latitude":60.1699,` +
			`"longitude":24.9384,` +
			`"speed_kmh":32.5,` +
			`"heading":120,` +
			`"accuracy_meters":4.2,` +
			`"recorded_at":"` +
			recordedAt.Format(time.RFC3339Nano) +
			`"` +
			`}`,
	)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips/"+tripID+"/locations",
		body,
	)

	c.Request.Header.Set(
		"Content-Type",
		"application/json",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: tripID,
		},
	}

	user := &models.User{}
	user.ID = userID

	middleware.SetCurrentUser(
		c,
		user,
	)

	return c, recorder
}

func TestRecordTripLocationPublishesRealtimeEventAfterSuccess(
	t *testing.T,
) {
	const (
		tripID   = "trip-1"
		driverID = "driver-user-1"
	)

	recordedAt := time.Now().
		UTC().
		Truncate(time.Millisecond)

	location := &models.TripLocation{
		ID:         "location-1",
		TripID:     tripID,
		DriverID:   driverID,
		Latitude:   60.1699,
		Longitude:  24.9384,
		RecordedAt: recordedAt,
	}

	service := &tripLocationRealtimeTestService{
		location: location,
	}

	broker := realtime.NewBroker()

	events, unsubscribe := broker.Subscribe(
		"trip:" + tripID,
	)
	defer unsubscribe()

	handler := NewTripHandlerWithRealtime(
		service,
		broker,
	)

	c, recorder := newTripLocationRealtimeTestContext(
		t,
		tripID,
		driverID,
		recordedAt,
	)

	handler.RecordTripLocation(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusCreated,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !service.called {
		t.Fatal(
			"expected RecordTripLocation service to be called",
		)
	}

	if service.tripID != tripID {
		t.Fatalf(
			"expected service trip ID %q, got %q",
			tripID,
			service.tripID,
		)
	}

	if service.actorUserID != driverID {
		t.Fatalf(
			"expected actor user ID %q, got %q",
			driverID,
			service.actorUserID,
		)
	}

	select {
	case event := <-events:
		if event.Type != "trip.location" {
			t.Fatalf(
				"expected event type %q, got %q",
				"trip.location",
				event.Type,
			)
		}

		publishedLocation, ok :=
			event.Data.(*models.TripLocation)

		if !ok {
			t.Fatalf(
				"expected event data type *models.TripLocation, got %T",
				event.Data,
			)
		}

		if publishedLocation != location {
			t.Fatal(
				"expected realtime event to contain the persisted location returned by the service",
			)
		}

		if publishedLocation.ID != "location-1" {
			t.Fatalf(
				"expected persisted location ID %q, got %q",
				"location-1",
				publishedLocation.ID,
			)
		}

		if publishedLocation.TripID != tripID {
			t.Fatalf(
				"expected published trip ID %q, got %q",
				tripID,
				publishedLocation.TripID,
			)
		}

		if publishedLocation.DriverID != driverID {
			t.Fatalf(
				"expected published driver ID %q, got %q",
				driverID,
				publishedLocation.DriverID,
			)
		}

	case <-time.After(time.Second):
		t.Fatal(
			"expected trip.location realtime event",
		)
	}
}

func TestRecordTripLocationDoesNotPublishWhenPersistenceFails(
	t *testing.T,
) {
	const (
		tripID   = "trip-1"
		driverID = "driver-user-1"
	)

	recordedAt := time.Now().
		UTC().
		Truncate(time.Millisecond)

	service := &tripLocationRealtimeTestService{
		err: errors.New("database write failed"),
	}

	broker := realtime.NewBroker()

	events, unsubscribe := broker.Subscribe(
		"trip:" + tripID,
	)
	defer unsubscribe()

	handler := NewTripHandlerWithRealtime(
		service,
		broker,
	)

	c, recorder := newTripLocationRealtimeTestContext(
		t,
		tripID,
		driverID,
		recordedAt,
	)

	handler.RecordTripLocation(c)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusInternalServerError,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !service.called {
		t.Fatal(
			"expected RecordTripLocation service to be called",
		)
	}

	select {
	case event := <-events:
		t.Fatalf(
			"expected no realtime event after persistence failure, got type=%q data=%v",
			event.Type,
			event.Data,
		)

	case <-time.After(100 * time.Millisecond):
		// Expected: failed persistence must not emit realtime state.
	}
}
