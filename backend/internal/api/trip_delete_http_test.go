package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type tripDeleteHTTPTestService struct {
	trip.Service

	err    error
	calls  int
	tripID string
	userID string
}

func (s *tripDeleteHTTPTestService) DeleteAuthorized(
	_ context.Context,
	tripID string,
	userID string,
) error {
	s.calls++
	s.tripID = tripID
	s.userID = userID
	return s.err
}

func TestTripDeleteHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID = "trip-delete-123"
		userID = "customer-delete-123"
		secret = "private-database-connection-details"
	)

	cases := []struct {
		name          string
		authenticated bool
		serviceErr    error
		wantStatus    int
		wantCalls     int
		wantSuccess   bool
		wantMessage   string
	}{
		{
			name:       "unauthenticated request",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "unauthorized deletion",
			authenticated: true,
			serviceErr:    trip.ErrTripMutationAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
		},
		{
			name:          "nonterminal trip",
			authenticated: true,
			serviceErr:    trip.ErrTripDeleteRequiresTerminalStatus,
			wantStatus:    http.StatusConflict,
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
			serviceErr:    errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
			wantMessage:   "Failed to delete trip",
		},
		{
			name:          "wrapped internal failure",
			authenticated: true,
			serviceErr:    fmt.Errorf("delete failed: %w", errors.New(secret)),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
			wantMessage:   "Failed to delete trip",
		},
		{
			name:          "successful deletion",
			authenticated: true,
			wantStatus:    http.StatusOK,
			wantCalls:     1,
			wantSuccess:   true,
			wantMessage:   "Trip deleted successfully",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripDeleteHTTPTestService{
				err: tc.serviceErr,
			}

			router := gin.New()

			router.DELETE(
				"/api/v1/trips/:id",
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
				NewTripHandler(service).DeleteTrip,
			)

			recorder := httptest.NewRecorder()

			router.ServeHTTP(
				recorder,
				httptest.NewRequest(
					http.MethodDelete,
					"/api/v1/trips/"+tripID,
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
						"trip ID = %q, want %q",
						service.tripID,
						tripID,
					)
				}

				if service.userID != userID {
					t.Fatalf(
						"user ID = %q, want %q",
						service.userID,
						userID,
					)
				}
			}

			body := recorder.Body.String()

			if strings.Contains(body, secret) {
				t.Fatalf(
					"internal error details disclosed: %s",
					body,
				)
			}

			var response struct {
				Success bool            `json:"success"`
				Message string          `json:"message"`
				Data    json.RawMessage `json:"data"`
				Error   json.RawMessage `json:"error"`
			}

			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&response,
			); err != nil {
				t.Fatalf(
					"invalid JSON response: %v; body=%s",
					err,
					body,
				)
			}

			if response.Success != tc.wantSuccess {
				t.Fatalf(
					"success = %t, want %t",
					response.Success,
					tc.wantSuccess,
				)
			}

			if tc.wantMessage != "" &&
				response.Message != tc.wantMessage {
				t.Fatalf(
					"message = %q, want %q",
					response.Message,
					tc.wantMessage,
				)
			}

			if len(response.Data) != 0 {
				t.Fatalf(
					"unexpected response data: %s",
					string(response.Data),
				)
			}

			if len(response.Error) != 0 {
				t.Fatalf(
					"unexpected error field: %s",
					string(response.Error),
				)
			}
		})
	}
}
