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

type tripStatusHTTPTestService struct {
	trip.Service

	updateErr error

	updateCalls int
	tripID      string
	status      string
	userID      string
}

func (s *tripStatusHTTPTestService) UpdateStatus(
	_ context.Context,
	tripID string,
	status string,
	userID string,
) error {
	s.updateCalls++
	s.tripID = tripID
	s.status = status
	s.userID = userID
	return s.updateErr
}

func TestTripStatusHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID = "trip-status-http-123"
		userID = "driver-status-http-456"
		secret = "private-trip-status-database-details"
	)

	validBody := `{"status":"DRIVER_EN_ROUTE"}`

	cases := []struct {
		name          string
		authenticated bool
		body          string
		updateErr     error
		wantStatus    int
		wantCalls     int
		wantSuccess   bool
		wantMessage   string
	}{
		{
			name:        "unauthenticated request",
			body:        validBody,
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authenticated user not found",
		},
		{
			name:          "missing status field",
			authenticated: true,
			body:          `{}`,
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Invalid request body",
		},
		{
			name:          "malformed JSON",
			authenticated: true,
			body:          `{"status":`,
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Invalid request body",
		},
		{
			name:          "invalid status value",
			authenticated: true,
			body:          `{"status":"UNKNOWN"}`,
			updateErr: fmt.Errorf(
				"%w: invalid trip status: UNKNOWN",
				trip.ErrTripStatusValidation,
			),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   1,
			wantMessage: "Invalid trip status or lifecycle transition",
		},
		{
			name:          "invalid lifecycle transition",
			authenticated: true,
			body:          validBody,
			updateErr: fmt.Errorf(
				"%w: invalid trip status transition",
				trip.ErrTripStatusValidation,
			),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   1,
			wantMessage: "Invalid trip status or lifecycle transition",
		},
		{
			name:          "unauthorized status update",
			authenticated: true,
			body:          validBody,
			updateErr:     trip.ErrTripStatusAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
			wantMessage:   "You are not authorized to update this trip's status",
		},
		{
			name:          "wrapped unauthorized status update",
			authenticated: true,
			body:          validBody,
			updateErr: fmt.Errorf(
				"status authorization failed: %w",
				trip.ErrTripStatusAccessDenied,
			),
			wantStatus:  http.StatusForbidden,
			wantCalls:   1,
			wantMessage: "You are not authorized to update this trip's status",
		},
		{
			name:          "missing trip",
			authenticated: true,
			body:          validBody,
			updateErr:     pgx.ErrNoRows,
			wantStatus:    http.StatusNotFound,
			wantCalls:     1,
			wantMessage:   "Trip not found",
		},
		{
			name:          "wrapped missing trip",
			authenticated: true,
			body:          validBody,
			updateErr: fmt.Errorf(
				"load trip: %w",
				pgx.ErrNoRows,
			),
			wantStatus:  http.StatusNotFound,
			wantCalls:   1,
			wantMessage: "Trip not found",
		},
		{
			name:          "internal status failure",
			authenticated: true,
			body:          validBody,
			updateErr:     errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
			wantMessage:   "Failed to update trip status",
		},
		{
			name:          "wrapped internal status failure",
			authenticated: true,
			body:          validBody,
			updateErr: fmt.Errorf(
				"status transaction failed: %w",
				errors.New(secret),
			),
			wantStatus:  http.StatusInternalServerError,
			wantCalls:   1,
			wantMessage: "Failed to update trip status",
		},
		{
			name:          "successful status update",
			authenticated: true,
			body:          validBody,
			wantStatus:    http.StatusOK,
			wantCalls:     1,
			wantSuccess:   true,
			wantMessage:   "Trip status updated successfully",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripStatusHTTPTestService{
				updateErr: tc.updateErr,
			}

			router := gin.New()

			router.PATCH(
				"/api/v1/trips/:id/status",
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
				NewTripHandler(service).UpdateTripStatus,
			)

			recorder := httptest.NewRecorder()

			request := httptest.NewRequest(
				http.MethodPatch,
				"/api/v1/trips/"+tripID+"/status",
				strings.NewReader(tc.body),
			)

			request.Header.Set(
				"Content-Type",
				"application/json",
			)

			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"HTTP status = %d, want %d; body=%s",
					recorder.Code,
					tc.wantStatus,
					recorder.Body.String(),
				)
			}

			if service.updateCalls != tc.wantCalls {
				t.Fatalf(
					"service calls = %d, want %d",
					service.updateCalls,
					tc.wantCalls,
				)
			}

			if strings.Contains(recorder.Body.String(), secret) {
				t.Fatalf(
					"internal error disclosed: %s",
					recorder.Body.String(),
				)
			}

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}

			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&response,
			); err != nil {
				t.Fatalf(
					"invalid JSON response: %v",
					err,
				)
			}

			if response.Success != tc.wantSuccess {
				t.Fatalf(
					"success = %t, want %t",
					response.Success,
					tc.wantSuccess,
				)
			}

			if response.Message != tc.wantMessage {
				t.Fatalf(
					"message = %q, want %q",
					response.Message,
					tc.wantMessage,
				)
			}

			if tc.wantCalls == 0 {
				if service.tripID != "" ||
					service.status != "" ||
					service.userID != "" {
					t.Fatal("service received unexpected arguments")
				}
				return
			}

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

			var requestBody struct {
				Status string `json:"status"`
			}

			if err := json.Unmarshal(
				[]byte(tc.body),
				&requestBody,
			); err != nil {
				t.Fatalf("invalid test request body: %v", err)
			}

			if service.status != requestBody.Status {
				t.Fatalf(
					"forwarded status = %q, want %q",
					service.status,
					requestBody.Status,
				)
			}
		})
	}
}
