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

type tripReadHTTPTestService struct {
	trip.Service

	getResult  *models.Trip
	getErr     error
	listResult []*models.Trip
	listErr    error

	getCalls  int
	listCalls int

	getTripID string
	getUserID string

	listUserID     string
	listCompanyID  string
	listBranchID   string
	listStatus     string
	listDriverID   string
	listCustomerID string
	listLimit      int
	listOffset     int
}

func (s *tripReadHTTPTestService) GetByIDAuthorized(
	_ context.Context,
	tripID string,
	userID string,
) (*models.Trip, error) {
	s.getCalls++
	s.getTripID = tripID
	s.getUserID = userID
	return s.getResult, s.getErr
}

func (s *tripReadHTTPTestService) ListAuthorized(
	_ context.Context,
	userID string,
	companyID string,
	branchID string,
	status string,
	driverID string,
	customerID string,
	limit int,
	offset int,
) ([]*models.Trip, error) {
	s.listCalls++
	s.listUserID = userID
	s.listCompanyID = companyID
	s.listBranchID = branchID
	s.listStatus = status
	s.listDriverID = driverID
	s.listCustomerID = customerID
	s.listLimit = limit
	s.listOffset = offset
	return s.listResult, s.listErr
}

func TestTripReadHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID = "trip-read-http-123"
		userID = "customer-read-http-456"
		secret = "private-trip-read-database-connection-details"
	)

	type testCase struct {
		name          string
		method        string
		path          string
		authenticated bool

		getErr  error
		listErr error

		wantStatus    int
		wantMessage   string
		wantGetCalls  int
		wantListCalls int
	}

	tests := []testCase{
		{
			name:        "get unauthenticated",
			method:      http.MethodGet,
			path:        "/api/v1/trips/" + tripID,
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authenticated user not found",
		},
		{
			name:          "get forbidden",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			getErr:        trip.ErrTripAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantMessage:   "You are not authorized to view this trip",
			wantGetCalls:  1,
		},
		{
			name:          "get wrapped forbidden",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			getErr:        fmt.Errorf("wrapped: %w", trip.ErrTripAccessDenied),
			wantStatus:    http.StatusForbidden,
			wantMessage:   "You are not authorized to view this trip",
			wantGetCalls:  1,
		},
		{
			name:          "get missing trip",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			getErr:        pgx.ErrNoRows,
			wantStatus:    http.StatusNotFound,
			wantMessage:   "Trip not found",
			wantGetCalls:  1,
		},
		{
			name:          "get wrapped missing trip",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			getErr:        fmt.Errorf("wrapped: %w", pgx.ErrNoRows),
			wantStatus:    http.StatusNotFound,
			wantMessage:   "Trip not found",
			wantGetCalls:  1,
		},
		{
			name:          "get internal failure",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			getErr:        errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "Failed to retrieve trip",
			wantGetCalls:  1,
		},
		{
			name:          "get wrapped internal failure",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			getErr:        fmt.Errorf("wrapped: %w", errors.New(secret)),
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "Failed to retrieve trip",
			wantGetCalls:  1,
		},
		{
			name:          "get success",
			method:        http.MethodGet,
			path:          "/api/v1/trips/" + tripID,
			authenticated: true,
			wantStatus:    http.StatusOK,
			wantGetCalls:  1,
		},
		{
			name:        "list unauthenticated",
			method:      http.MethodGet,
			path:        "/api/v1/trips",
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authenticated user not found",
		},
		{
			name:          "list invalid limit",
			method:        http.MethodGet,
			path:          "/api/v1/trips?limit=invalid",
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Invalid limit",
		},
		{
			name:          "list invalid offset",
			method:        http.MethodGet,
			path:          "/api/v1/trips?offset=invalid",
			authenticated: true,
			wantStatus:    http.StatusBadRequest,
			wantMessage:   "Invalid offset",
		},
		{
			name:          "list forbidden",
			method:        http.MethodGet,
			path:          "/api/v1/trips",
			authenticated: true,
			listErr:       trip.ErrTripAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantMessage:   "You are not authorized to list trips",
			wantListCalls: 1,
		},
		{
			name:          "list wrapped forbidden",
			method:        http.MethodGet,
			path:          "/api/v1/trips",
			authenticated: true,
			listErr:       fmt.Errorf("wrapped: %w", trip.ErrTripAccessDenied),
			wantStatus:    http.StatusForbidden,
			wantMessage:   "You are not authorized to list trips",
			wantListCalls: 1,
		},
		{
			name:          "list internal failure",
			method:        http.MethodGet,
			path:          "/api/v1/trips",
			authenticated: true,
			listErr:       errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "Failed to retrieve trips",
			wantListCalls: 1,
		},
		{
			name:          "list wrapped internal failure",
			method:        http.MethodGet,
			path:          "/api/v1/trips",
			authenticated: true,
			listErr:       fmt.Errorf("wrapped: %w", errors.New(secret)),
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "Failed to retrieve trips",
			wantListCalls: 1,
		},
		{
			name:          "list success with filters",
			method:        http.MethodGet,
			path:          "/api/v1/trips?company_id=company-1&branch_id=branch-1&status=completed&driver_id=driver-1&customer_id=customer-1&limit=12&offset=4",
			authenticated: true,
			wantStatus:    http.StatusOK,
			wantListCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripReadHTTPTestService{
				getResult:  &models.Trip{},
				listResult: []*models.Trip{},
				getErr:     tc.getErr,
				listErr:    tc.listErr,
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

			router.GET("/api/v1/trips", handler.ListTrips)
			router.GET("/api/v1/trips/:id", handler.GetTrip)

			recorder := httptest.NewRecorder()

			request := httptest.NewRequest(
				tc.method,
				tc.path,
				nil,
			)

			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"HTTP status: got %d, want %d; body=%s",
					recorder.Code,
					tc.wantStatus,
					recorder.Body.String(),
				)
			}

			var body map[string]any

			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode HTTP response: %v", err)
			}

			if tc.wantMessage != "" {
				if body["message"] != tc.wantMessage {
					t.Errorf(
						"message: got %v, want %q",
						body["message"],
						tc.wantMessage,
					)
				}
			}

			if strings.Contains(recorder.Body.String(), secret) {
				t.Errorf("internal error details leaked to HTTP client")
			}

			if _, exposed := body["error"]; exposed {
				t.Errorf("HTTP response exposes internal error field")
			}

			if service.getCalls != tc.wantGetCalls {
				t.Errorf(
					"GetByIDAuthorized calls: got %d, want %d",
					service.getCalls,
					tc.wantGetCalls,
				)
			}

			if service.listCalls != tc.wantListCalls {
				t.Errorf(
					"ListAuthorized calls: got %d, want %d",
					service.listCalls,
					tc.wantListCalls,
				)
			}

			if service.getCalls == 1 {
				if service.getTripID != tripID || service.getUserID != userID {
					t.Errorf(
						"GetByIDAuthorized arguments: trip=%q user=%q",
						service.getTripID,
						service.getUserID,
					)
				}
			}

			if service.listCalls == 1 {
				if service.listUserID != userID {
					t.Errorf(
						"ListAuthorized user: got %q, want %q",
						service.listUserID,
						userID,
					)
				}

				if tc.name == "list success with filters" {
					if service.listCompanyID != "company-1" ||
						service.listBranchID != "branch-1" ||
						service.listStatus != "completed" ||
						service.listDriverID != "driver-1" ||
						service.listCustomerID != "customer-1" ||
						service.listLimit != 12 ||
						service.listOffset != 4 {
						t.Errorf(
							"ListAuthorized filters incorrect: %+v",
							service,
						)
					}
				} else if service.listLimit != 50 || service.listOffset != 0 {
					t.Errorf(
						"default pagination: limit=%d offset=%d",
						service.listLimit,
						service.listOffset,
					)
				}
			}

			if recorder.Code == http.StatusOK {
				if body["success"] != true {
					t.Errorf("successful response missing success=true")
				}
				if _, ok := body["data"]; !ok {
					t.Errorf("successful response missing data")
				}
			}
		})
	}
}
