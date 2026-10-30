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

type tripAssignHTTPTestService struct {
	trip.Service

	assignErr error

	assignCalls int
	tripID      string
	driverID    string
	vehicleID   string
	userID      string
}

func (s *tripAssignHTTPTestService) AssignDriverAuthorized(
	_ context.Context,
	tripID string,
	driverID string,
	vehicleID string,
	userID string,
) error {
	s.assignCalls++
	s.tripID = tripID
	s.driverID = driverID
	s.vehicleID = vehicleID
	s.userID = userID
	return s.assignErr
}

func TestTripAssignHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID    = "trip-assign-http-123"
		driverID  = "driver-assign-http-456"
		vehicleID = "vehicle-assign-http-789"
		userID    = "operator-assign-http-321"
		secret    = "private-assignment-database-details"
	)

	validBody := fmt.Sprintf(
		`{"driver_id":%q,"vehicle_id":%q}`,
		driverID,
		vehicleID,
	)

	cases := []struct {
		name          string
		authenticated bool
		body          string
		assignErr     error
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
			name:          "malformed JSON",
			authenticated: true,
			body:          "{",
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Invalid request body",
		},
		{
			name:          "missing driver ID",
			authenticated: true,
			body:          `{"vehicle_id":"vehicle-123"}`,
			assignErr:     trip.ErrTripAssignmentValidation,
			wantStatus:    http.StatusBadRequest,
			wantCalls:     0,
			wantMessage:   "Invalid request body",
		},
		{
			name:          "missing vehicle ID",
			authenticated: true,
			body:          `{"driver_id":"driver-123"}`,
			assignErr:     trip.ErrTripAssignmentValidation,
			wantStatus:    http.StatusBadRequest,
			wantCalls:     0,
			wantMessage:   "Invalid request body",
		},
		{
			name:          "invalid trip lifecycle",
			authenticated: true,
			body:          validBody,
			assignErr: fmt.Errorf(
				"%w: trip status COMPLETED",
				trip.ErrTripAssignmentValidation,
			),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   1,
			wantMessage: "Invalid trip driver assignment",
		},
		{
			name:          "unauthorized assignment",
			authenticated: true,
			body:          validBody,
			assignErr:     trip.ErrTripMutationAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
			wantMessage:   "You are not authorized to assign drivers to trips",
		},
		{
			name:          "wrapped unauthorized assignment",
			authenticated: true,
			body:          validBody,
			assignErr: fmt.Errorf(
				"tenant authorization: %w",
				trip.ErrTripMutationAccessDenied,
			),
			wantStatus:  http.StatusForbidden,
			wantCalls:   1,
			wantMessage: "You are not authorized to assign drivers to trips",
		},
		{
			name:          "missing trip",
			authenticated: true,
			body:          validBody,
			assignErr:     pgx.ErrNoRows,
			wantStatus:    http.StatusNotFound,
			wantCalls:     1,
			wantMessage:   "Trip not found",
		},
		{
			name:          "wrapped missing trip",
			authenticated: true,
			body:          validBody,
			assignErr: fmt.Errorf(
				"assignment lookup: %w",
				pgx.ErrNoRows,
			),
			wantStatus:  http.StatusNotFound,
			wantCalls:   1,
			wantMessage: "Trip not found",
		},
		{
			name:          "internal database failure",
			authenticated: true,
			body:          validBody,
			assignErr:     errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
			wantMessage:   "Failed to assign driver to trip",
		},
		{
			name:          "wrapped internal failure",
			authenticated: true,
			body:          validBody,
			assignErr: fmt.Errorf(
				"assignment transaction: %w",
				errors.New(secret),
			),
			wantStatus:  http.StatusInternalServerError,
			wantCalls:   1,
			wantMessage: "Failed to assign driver to trip",
		},
		{
			name:          "successful assignment",
			authenticated: true,
			body:          validBody,
			wantStatus:    http.StatusOK,
			wantCalls:     1,
			wantSuccess:   true,
			wantMessage:   "Driver and vehicle assigned successfully",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripAssignHTTPTestService{
				assignErr: tc.assignErr,
			}

			router := gin.New()

			router.POST(
				"/api/v1/trips/:id/assign-driver",
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
				NewTripHandler(service).AssignDriver,
			)

			recorder := httptest.NewRecorder()

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/trips/"+tripID+"/assign-driver",
				strings.NewReader(tc.body),
			)
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"HTTP status = %d, want %d; body=%s",
					recorder.Code,
					tc.wantStatus,
					recorder.Body.String(),
				)
			}

			if service.assignCalls != tc.wantCalls {
				t.Fatalf(
					"assignment calls = %d, want %d",
					service.assignCalls,
					tc.wantCalls,
				)
			}

			if strings.Contains(recorder.Body.String(), secret) {
				t.Fatalf(
					"internal assignment details disclosed: %s",
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
					service.driverID != "" ||
					service.vehicleID != "" ||
					service.userID != "" {
					t.Fatal("unexpected assignment service arguments")
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

			var expectedDriverID, expectedVehicleID string

			if err := json.Unmarshal(
				[]byte(tc.body),
				&struct {
					DriverID  *string `json:"driver_id"`
					VehicleID *string `json:"vehicle_id"`
				}{},
			); err != nil {
				t.Fatalf("invalid test fixture: %v", err)
			}

			var payload struct {
				DriverID  string `json:"driver_id"`
				VehicleID string `json:"vehicle_id"`
			}

			if err := json.Unmarshal(
				[]byte(tc.body),
				&payload,
			); err != nil {
				t.Fatalf("invalid fixture JSON: %v", err)
			}

			expectedDriverID = payload.DriverID
			expectedVehicleID = payload.VehicleID

			if service.driverID != expectedDriverID {
				t.Fatalf(
					"driver ID = %q, want %q",
					service.driverID,
					expectedDriverID,
				)
			}

			if service.vehicleID != expectedVehicleID {
				t.Fatalf(
					"vehicle ID = %q, want %q",
					service.vehicleID,
					expectedVehicleID,
				)
			}
		})
	}
}
