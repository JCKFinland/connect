package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type httpDiscoveryAuthorizer struct {
	request *models.RideRequest
	err     error
	calls   int
}

func (a *httpDiscoveryAuthorizer) GetByIDAuthorized(
	_ context.Context, _, _ string,
) (*models.RideRequest, error) {
	a.calls++
	return a.request, a.err
}

type httpDiscoveryTrips struct {
	repository.TripRepository
	tripID      string
	result      *models.Trip
	lookupErr   error
	getErr      error
	lookupCalls int
	getCalls    int
}

func (r *httpDiscoveryTrips) GetIDByRideRequestID(
	_ context.Context, _ string,
) (string, error) {
	r.lookupCalls++
	return r.tripID, r.lookupErr
}

func (r *httpDiscoveryTrips) GetByID(
	_ context.Context, _ string,
) (*models.Trip, error) {
	r.getCalls++
	return r.result, r.getErr
}

type httpDiscoveryRoles struct {
	repository.UserRoleRepository
}

func (r *httpDiscoveryRoles) GetUserRoles(
	_ context.Context, _ string,
) ([]string, error) {
	return []string{"CUSTOMER"}, nil
}

func TestTripDiscoveryHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		requestID  = "request-123"
		customerID = "customer-123"
		tripID     = "trip-123"
	)

	tests := []struct {
		name            string
		authenticated   bool
		authorizerErr   error
		lookupErr       error
		getErr          error
		tripCustomer    string
		expectedCode    int
		expectedLookups int
	}{
		{
			name:            "owned trip",
			authenticated:   true,
			tripCustomer:    customerID,
			expectedCode:    http.StatusOK,
			expectedLookups: 1,
		},
		{
			name:         "unauthenticated",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:          "unauthorized ride request",
			authenticated: true,
			authorizerErr: ride_request.ErrRideRequestAccessDenied,
			expectedCode:  http.StatusForbidden,
		},
		{
			name:            "unauthorized trip",
			authenticated:   true,
			tripCustomer:    "another-customer",
			expectedCode:    http.StatusForbidden,
			expectedLookups: 1,
		},
		{
			name:            "no trip yet",
			authenticated:   true,
			lookupErr:       pgx.ErrNoRows,
			expectedCode:    http.StatusNotFound,
			expectedLookups: 1,
		},
		{
			name:            "unexpected repository failure",
			authenticated:   true,
			lookupErr:       errors.New("database unavailable"),
			expectedCode:    http.StatusInternalServerError,
			expectedLookups: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := &httpDiscoveryAuthorizer{
				request: &models.RideRequest{
					BaseModel:  models.BaseModel{ID: requestID},
					CustomerID: customerID,
				},
				err: tt.authorizerErr,
			}

			trips := &httpDiscoveryTrips{
				tripID: tripID,
				result: &models.Trip{
					BaseModel:     models.BaseModel{ID: tripID},
					RideRequestID: requestID,
					CustomerID:    tt.tripCustomer,
				},
				lookupErr: tt.lookupErr,
				getErr:    tt.getErr,
			}

			service := trip.NewService(trip.Dependencies{
				Trips:             trips,
				RideRequestAccess: authorizer,
				UserRoles:         &httpDiscoveryRoles{},
			})

			router := gin.New()
			router.GET(
				"/api/v1/ride-requests/:id/trip",
				func(c *gin.Context) {
					if tt.authenticated {
						middleware.SetCurrentUser(c, &models.User{
							BaseModel: models.BaseModel{ID: customerID},
						})
					}
					c.Next()
				},
				NewTripHandler(service).GetTripByRideRequest,
			)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(
				recorder,
				httptest.NewRequest(
					http.MethodGet,
					"/api/v1/ride-requests/"+requestID+"/trip",
					nil,
				),
			)

			if recorder.Code != tt.expectedCode {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tt.expectedCode,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if trips.lookupCalls != tt.expectedLookups {
				t.Fatalf(
					"expected %d trip lookups, got %d",
					tt.expectedLookups,
					trips.lookupCalls,
				)
			}

			if !tt.authenticated && authorizer.calls != 0 {
				t.Fatal("unauthenticated request reached service")
			}

			if tt.authorizerErr != nil && trips.getCalls != 0 {
				t.Fatal("unauthorized ride request exposed trip lookup")
			}

			var response struct {
				Success bool            `json:"success"`
				Data    json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if tt.expectedCode == http.StatusOK {
				if !response.Success || len(response.Data) == 0 {
					t.Fatalf("missing successful trip response: %s", recorder.Body.String())
				}
			} else if response.Success || len(response.Data) != 0 {
				t.Fatalf("failed request exposed trip data: %s", recorder.Body.String())
			}
		})
	}
}

func TestTripDiscoveryRouteRequiresAuthentication(t *testing.T) {
	source, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes: %v", err)
	}

	sectionStart := strings.Index(
		string(source),
		`rideRequests := v1.Group("/ride-requests")`,
	)
	if sectionStart < 0 {
		t.Fatal("ride-request route group missing")
	}

	section := string(source)[sectionStart:]

	for _, expected := range []string{
		`rideRequests.Use(authMiddleware.RequireAuth())`,
		`"/:id/trip"`,
		`tripHandler.GetTripByRideRequest`,
	} {
		if !strings.Contains(section, expected) {
			t.Fatalf("missing authenticated route requirement: %s", expected)
		}
	}
}
