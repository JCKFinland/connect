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

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type tripCreateHTTPTestService struct {
	trip.Service

	createErr error

	createCalls  int
	createUserID string
	createdTrip  *models.Trip
}

func (s *tripCreateHTTPTestService) CreateAuthorized(
	_ context.Context,
	newTrip *models.Trip,
	userID string,
) error {
	s.createCalls++
	s.createUserID = userID
	s.createdTrip = newTrip
	return s.createErr
}

func TestTripCreateHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		userID = "create-http-admin-123"
		secret = "private-create-database-connection-details"
	)

	validBody := `{
        "ride_request_id": "ride-request-create-123",
        "customer_id": "customer-create-123",
        "driver_id": "driver-create-123",
        "vehicle_id": "vehicle-create-123",
        "company_id": "company-create-123",
        "branch_id": "branch-create-123",
        "fleet_id": "fleet-create-123"
    }`

	cases := []struct {
		name          string
		authenticated bool
		body          string
		createErr     error
		wantStatus    int
		wantCalls     int
		wantSuccess   bool
	}{
		{
			name:       "unauthenticated request",
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "invalid request body",
			authenticated: true,
			body:          `{}`,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "unauthorized creation",
			authenticated: true,
			body:          validBody,
			createErr:     trip.ErrTripMutationAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
		},
		{
			name:          "wrapped unauthorized creation",
			authenticated: true,
			body:          validBody,
			createErr: fmt.Errorf(
				"authorization rejected: %w",
				trip.ErrTripMutationAccessDenied,
			),
			wantStatus: http.StatusForbidden,
			wantCalls:  1,
		},
		{
			name:          "internal service failure",
			authenticated: true,
			body:          validBody,
			createErr:     errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
		},
		{
			name:          "wrapped internal service failure",
			authenticated: true,
			body:          validBody,
			createErr: fmt.Errorf(
				"create transaction failed: %w",
				errors.New(secret),
			),
			wantStatus: http.StatusInternalServerError,
			wantCalls:  1,
		},
		{
			name:          "successful creation",
			authenticated: true,
			body:          validBody,
			wantStatus:    http.StatusCreated,
			wantCalls:     1,
			wantSuccess:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripCreateHTTPTestService{
				createErr: tc.createErr,
			}

			router := gin.New()

			router.POST(
				"/api/v1/trips",
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
				NewTripHandler(service).CreateTrip,
			)

			recorder := httptest.NewRecorder()

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/trips",
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

			if service.createCalls != tc.wantCalls {
				t.Fatalf(
					"create calls = %d, want %d",
					service.createCalls,
					tc.wantCalls,
				)
			}

			if strings.Contains(
				recorder.Body.String(),
				secret,
			) {
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

			if tc.wantCalls == 0 {
				if service.createdTrip != nil {
					t.Fatal(
						"service received trip unexpectedly",
					)
				}
				return
			}

			if service.createUserID != userID {
				t.Fatalf(
					"create user ID = %q, want %q",
					service.createUserID,
					userID,
				)
			}

			if service.createdTrip == nil {
				t.Fatal(
					"service did not receive trip",
				)
			}

			created := service.createdTrip

			if created.RideRequestID != "ride-request-create-123" ||
				created.CustomerID != "customer-create-123" ||
				created.DriverID != "driver-create-123" ||
				created.VehicleID != "vehicle-create-123" ||
				created.CompanyID != "company-create-123" ||
				created.BranchID != "branch-create-123" ||
				created.FleetID != "fleet-create-123" {
				t.Fatalf(
					"incorrect trip request mapping: %+v",
					created,
				)
			}

			if tc.wantSuccess &&
				response.Message != "Trip created successfully" {
				t.Fatalf(
					"unexpected success message: %q",
					response.Message,
				)
			}

			if tc.wantStatus == http.StatusInternalServerError &&
				response.Message != "Failed to create trip" {
				t.Fatalf(
					"unexpected internal error message: %q",
					response.Message,
				)
			}
		})
	}
}
