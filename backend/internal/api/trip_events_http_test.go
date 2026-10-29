package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type tripEventsHTTPTestService struct {
	trip.Service

	events []*models.TripEvent
	err    error

	calls  int
	tripID string
	userID string
}

func (s *tripEventsHTTPTestService) ListEvents(
	_ context.Context,
	tripID string,
	userID string,
) ([]*models.TripEvent, error) {
	s.calls++
	s.tripID = tripID
	s.userID = userID

	if s.err != nil {
		return nil, s.err
	}

	return s.events, nil
}

func TestTripEventsHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID         = "trip-events-123"
		userID         = "customer-events-123"
		internalSecret = "private-database-connection-details"
	)

	cases := []struct {
		name          string
		authenticated bool
		serviceErr    error
		events        []*models.TripEvent
		wantStatus    int
		wantCalls     int
		wantSuccess   bool
	}{
		{
			name:       "unauthenticated request",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "unauthorized trip access",
			authenticated: true,
			serviceErr:    trip.ErrTripEventAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
		},
		{
			name:          "missing trip",
			authenticated: true,
			serviceErr:    pgx.ErrNoRows,
			wantStatus:    http.StatusNotFound,
			wantCalls:     1,
		},
		{
			name:          "internal service failure",
			authenticated: true,
			serviceErr:    errors.New(internalSecret),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
		},
		{
			name:          "authorized event retrieval",
			authenticated: true,
			events:        []*models.TripEvent{},
			wantStatus:    http.StatusOK,
			wantCalls:     1,
			wantSuccess:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripEventsHTTPTestService{
				events: tc.events,
				err:    tc.serviceErr,
			}

			router := gin.New()

			router.GET(
				"/api/v1/trips/:id/events",
				func(c *gin.Context) {
					if tc.authenticated {
						middleware.SetCurrentUser(
							c,
							&models.User{
								BaseModel: models.BaseModel{
									ID: userID,
								},
							},
						)
					}
					c.Next()
				},
				NewTripHandler(service).ListTripEvents,
			)

			recorder := httptest.NewRecorder()

			router.ServeHTTP(
				recorder,
				httptest.NewRequest(
					http.MethodGet,
					"/api/v1/trips/"+tripID+"/events",
					nil,
				),
			)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"HTTP status = %d, want %d; body=%s",
					recorder.Code,
					tc.wantStatus,
					recorder.Body.String(),
				)
			}

			if service.calls != tc.wantCalls {
				t.Fatalf(
					"service calls = %d, want %d",
					service.calls,
					tc.wantCalls,
				)
			}

			if tc.wantCalls > 0 {
				if service.tripID != tripID {
					t.Fatalf(
						"forwarded trip ID = %q, want %q",
						service.tripID,
						tripID,
					)
				}

				if service.userID != userID {
					t.Fatalf(
						"forwarded user ID = %q, want %q",
						service.userID,
						userID,
					)
				}
			}

			if strings.Contains(
				recorder.Body.String(),
				internalSecret,
			) {
				t.Fatalf(
					"internal error details exposed: %s",
					recorder.Body.String(),
				)
			}

			var response struct {
				Success bool            `json:"success"`
				Message string          `json:"message"`
				Data    json.RawMessage `json:"data"`
			}

			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&response,
			); err != nil {
				t.Fatalf(
					"invalid JSON response: %v; body=%s",
					err,
					recorder.Body.String(),
				)
			}

			if response.Success != tc.wantSuccess {
				t.Fatalf(
					"response success = %t, want %t",
					response.Success,
					tc.wantSuccess,
				)
			}

			if tc.wantSuccess {
				if len(response.Data) == 0 {
					t.Fatal("successful response missing event data")
				}

				var events []models.TripEvent

				if err := json.Unmarshal(
					response.Data,
					&events,
				); err != nil {
					t.Fatalf(
						"invalid event data: %v",
						err,
					)
				}

				if events == nil {
					t.Fatal("successful response returned null events")
				}
			} else {
				if len(response.Data) != 0 {
					t.Fatalf(
						"failure response exposed event data: %s",
						string(response.Data),
					)
				}
			}

			if tc.wantStatus == http.StatusInternalServerError {
				if response.Message != "Failed to retrieve trip events" {
					t.Fatalf(
						"unexpected internal error message: %q",
						response.Message,
					)
				}
			}
		})
	}
}
