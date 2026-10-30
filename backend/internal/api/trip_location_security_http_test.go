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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type tripLocationSecurityHTTPService struct {
	trip.Service

	recordErr error
	listErr   error

	recordCalls int
	listCalls   int

	recordTripID string
	recordUserID string
	listTripID   string
	listUserID   string
}

func (s *tripLocationSecurityHTTPService) RecordTripLocation(
	_ context.Context,
	tripID string,
	userID string,
	_ trip.RecordLocationRequest,
) (*models.TripLocation, error) {
	s.recordCalls++
	s.recordTripID = tripID
	s.recordUserID = userID

	if s.recordErr != nil {
		return nil, s.recordErr
	}

	return &models.TripLocation{}, nil
}

func (s *tripLocationSecurityHTTPService) ListTripLocations(
	_ context.Context,
	tripID string,
	userID string,
) ([]*models.TripLocation, error) {
	s.listCalls++
	s.listTripID = tripID
	s.listUserID = userID

	if s.listErr != nil {
		return nil, s.listErr
	}

	return []*models.TripLocation{}, nil
}

func TestTripLocationHTTPSecurityBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID = "trip-location-security-123"
		userID = "driver-location-security-456"
		secret = "private-location-database-connection-details"
	)

	type testCase struct {
		name          string
		method        string
		authenticated bool
		body          string

		recordErr error
		listErr   error

		wantStatus      int
		wantMessage     string
		wantRecordCalls int
		wantListCalls   int
	}

	validBody := fmt.Sprintf(
		`{"latitude":60.2,"longitude":24.7,"accuracy_meters":5,"recorded_at":%q}`,
		time.Now().UTC().Format(time.RFC3339),
	)

	tests := []testCase{
		{
			name:        "record unauthenticated",
			method:      http.MethodPost,
			body:        validBody,
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authenticated user not found",
		},
		{
			name:          "record invalid JSON",
			method:        http.MethodPost,
			authenticated: true,
			body:          "{",
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Invalid request body",
		},
		{
			name:          "record missing latitude",
			method:        http.MethodPost,
			authenticated: true,
			body:          `{"longitude":24.7,"accuracy_meters":5,"recorded_at":"2026-10-30T06:00:00Z"}`,
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "latitude is required",
		},
		{
			name:            "record forbidden",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			recordErr:       trip.ErrTripLocationAccessDenied,
			wantStatus:      http.StatusForbidden,
			wantMessage:     "You are not authorized to record location for this trip",
			wantRecordCalls: 1,
		},
		{
			name:            "record wrapped forbidden",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			recordErr:       fmt.Errorf("wrapped: %w", trip.ErrTripLocationAccessDenied),
			wantStatus:      http.StatusForbidden,
			wantMessage:     "You are not authorized to record location for this trip",
			wantRecordCalls: 1,
		},
		{
			name:            "record trip not in progress",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			recordErr:       trip.ErrTripNotInProgress,
			wantStatus:      http.StatusConflict,
			wantMessage:     "Trip is not in progress",
			wantRecordCalls: 1,
		},
		{
			name:            "record missing trip",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			recordErr:       fmt.Errorf("wrapped: %w", pgx.ErrNoRows),
			wantStatus:      http.StatusNotFound,
			wantMessage:     "Trip not found",
			wantRecordCalls: 1,
		},
		{
			name:            "record internal error",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			recordErr:       errors.New(secret),
			wantStatus:      http.StatusInternalServerError,
			wantMessage:     "Failed to record trip location",
			wantRecordCalls: 1,
		},
		{
			name:            "record wrapped internal error",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			recordErr:       fmt.Errorf("wrapped: %w", errors.New(secret)),
			wantStatus:      http.StatusInternalServerError,
			wantMessage:     "Failed to record trip location",
			wantRecordCalls: 1,
		},
		{
			name:            "record success",
			method:          http.MethodPost,
			authenticated:   true,
			body:            validBody,
			wantStatus:      http.StatusCreated,
			wantMessage:     "Trip location recorded successfully",
			wantRecordCalls: 1,
		},
		{
			name:        "list unauthenticated",
			method:      http.MethodGet,
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authenticated user not found",
		},
		{
			name:          "list forbidden",
			method:        http.MethodGet,
			authenticated: true,
			listErr:       trip.ErrTripAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantMessage:   "You are not authorized to view locations for this trip",
			wantListCalls: 1,
		},
		{
			name:          "list wrapped forbidden",
			method:        http.MethodGet,
			authenticated: true,
			listErr:       fmt.Errorf("wrapped: %w", trip.ErrTripAccessDenied),
			wantStatus:    http.StatusForbidden,
			wantMessage:   "You are not authorized to view locations for this trip",
			wantListCalls: 1,
		},
		{
			name:          "list missing trip",
			method:        http.MethodGet,
			authenticated: true,
			listErr:       fmt.Errorf("wrapped: %w", repository.ErrNotFound),
			wantStatus:    http.StatusNotFound,
			wantMessage:   "Trip not found",
			wantListCalls: 1,
		},
		{
			name:          "list internal error",
			method:        http.MethodGet,
			authenticated: true,
			listErr:       errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "Failed to retrieve trip locations",
			wantListCalls: 1,
		},
		{
			name:          "list wrapped internal error",
			method:        http.MethodGet,
			authenticated: true,
			listErr:       fmt.Errorf("wrapped: %w", errors.New(secret)),
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "Failed to retrieve trip locations",
			wantListCalls: 1,
		},
		{
			name:          "list success",
			method:        http.MethodGet,
			authenticated: true,
			wantStatus:    http.StatusOK,
			wantListCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripLocationSecurityHTTPService{
				recordErr: tc.recordErr,
				listErr:   tc.listErr,
			}

			router := gin.New()

			router.Use(func(c *gin.Context) {
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
			})

			handler := NewTripHandler(service)

			router.POST(
				"/api/v1/trips/:id/locations",
				handler.RecordTripLocation,
			)
			router.GET(
				"/api/v1/trips/:id/locations",
				handler.ListTripLocations,
			)

			path := "/api/v1/trips/" + tripID + "/locations"

			request := httptest.NewRequest(
				tc.method,
				path,
				strings.NewReader(tc.body),
			)

			if tc.method == http.MethodPost {
				request.Header.Set("Content-Type", "application/json")
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"HTTP status: got %d, want %d; body=%s",
					recorder.Code,
					tc.wantStatus,
					recorder.Body.String(),
				)
			}

			var response map[string]any

			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&response,
			); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if tc.wantMessage != "" {
				if response["message"] != tc.wantMessage {
					t.Errorf(
						"message: got %v, want %q",
						response["message"],
						tc.wantMessage,
					)
				}
			}

			if strings.Contains(recorder.Body.String(), secret) {
				t.Error("internal error details leaked to HTTP client")
			}

			if recorder.Code == http.StatusInternalServerError {
				if _, exposed := response["error"]; exposed {
					t.Error("HTTP 500 response exposes error field")
				}
			}

			if service.recordCalls != tc.wantRecordCalls {
				t.Errorf(
					"record calls: got %d, want %d",
					service.recordCalls,
					tc.wantRecordCalls,
				)
			}

			if service.listCalls != tc.wantListCalls {
				t.Errorf(
					"list calls: got %d, want %d",
					service.listCalls,
					tc.wantListCalls,
				)
			}

			if service.recordCalls == 1 {
				if service.recordTripID != tripID ||
					service.recordUserID != userID {
					t.Errorf(
						"record service arguments: trip=%q user=%q",
						service.recordTripID,
						service.recordUserID,
					)
				}
			}

			if service.listCalls == 1 {
				if service.listTripID != tripID ||
					service.listUserID != userID {
					t.Errorf(
						"list service arguments: trip=%q user=%q",
						service.listTripID,
						service.listUserID,
					)
				}
			}

			if recorder.Code == http.StatusOK ||
				recorder.Code == http.StatusCreated {
				if response["success"] != true {
					t.Error("successful response missing success=true")
				}
				if _, exists := response["data"]; !exists {
					t.Error("successful response missing data")
				}
			}
		})
	}
}
