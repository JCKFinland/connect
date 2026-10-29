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

type tripUpdateHTTPTestService struct {
	trip.Service

	lookupErr error
	updateErr error

	lookupCalls int
	updateCalls int

	lookupTripID string
	lookupUserID string
	updateUserID string
}

func (s *tripUpdateHTTPTestService) GetByIDAuthorized(
	_ context.Context,
	tripID string,
	userID string,
) (*models.Trip, error) {
	s.lookupCalls++
	s.lookupTripID = tripID
	s.lookupUserID = userID

	if s.lookupErr != nil {
		return nil, s.lookupErr
	}

	return &models.Trip{
		BaseModel: models.BaseModel{ID: tripID},
	}, nil
}

func (s *tripUpdateHTTPTestService) UpdateAuthorized(
	_ context.Context,
	_ *models.Trip,
	userID string,
) error {
	s.updateCalls++
	s.updateUserID = userID
	return s.updateErr
}

func TestTripUpdateHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID = "trip-update-123"
		userID = "customer-update-123"
		secret = "private-database-connection-details"
	)

	cases := []struct {
		name          string
		authenticated bool
		lookupErr     error
		updateErr     error
		wantStatus    int
		wantLookups   int
		wantUpdates   int
		wantSuccess   bool
	}{
		{
			name:       "unauthenticated request",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "unauthorized lookup",
			authenticated: true,
			lookupErr:     trip.ErrTripAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantLookups:   1,
		},
		{
			name:          "missing trip",
			authenticated: true,
			lookupErr:     pgx.ErrNoRows,
			wantStatus:    http.StatusNotFound,
			wantLookups:   1,
		},
		{
			name:          "internal lookup failure",
			authenticated: true,
			lookupErr:     errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantLookups:   1,
		},
		{
			name:          "unauthorized mutation",
			authenticated: true,
			updateErr:     trip.ErrTripMutationAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantLookups:   1,
			wantUpdates:   1,
		},
		{
			name:          "internal mutation failure",
			authenticated: true,
			updateErr:     errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantLookups:   1,
			wantUpdates:   1,
		},
		{
			name:          "wrapped internal mutation failure",
			authenticated: true,
			updateErr:     fmt.Errorf("update transaction failed: %w", errors.New(secret)),
			wantStatus:    http.StatusInternalServerError,
			wantLookups:   1,
			wantUpdates:   1,
		},
		{
			name:          "successful update",
			authenticated: true,
			wantStatus:    http.StatusOK,
			wantLookups:   1,
			wantUpdates:   1,
			wantSuccess:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripUpdateHTTPTestService{
				lookupErr: tc.lookupErr,
				updateErr: tc.updateErr,
			}

			router := gin.New()

			router.PUT(
				"/api/v1/trips/:id",
				func(c *gin.Context) {
					if tc.authenticated {
						middleware.SetCurrentUser(
							c,
							&models.User{
								BaseModel: models.BaseModel{ID: userID},
							},
						)
					}
					c.Next()
				},
				NewTripHandler(service).UpdateTrip,
			)

			recorder := httptest.NewRecorder()

			request := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/trips/"+tripID,
				strings.NewReader(`{}`),
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

			if service.lookupCalls != tc.wantLookups {
				t.Fatalf(
					"lookup calls = %d, want %d",
					service.lookupCalls,
					tc.wantLookups,
				)
			}

			if service.updateCalls != tc.wantUpdates {
				t.Fatalf(
					"update calls = %d, want %d",
					service.updateCalls,
					tc.wantUpdates,
				)
			}

			if tc.wantLookups > 0 {
				if service.lookupTripID != tripID {
					t.Fatalf("incorrect lookup trip ID: %q", service.lookupTripID)
				}
				if service.lookupUserID != userID {
					t.Fatalf("incorrect lookup user ID: %q", service.lookupUserID)
				}
			}

			if tc.wantUpdates > 0 && service.updateUserID != userID {
				t.Fatalf("incorrect update user ID: %q", service.updateUserID)
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

			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}

			if response.Success != tc.wantSuccess {
				t.Fatalf(
					"success = %t, want %t",
					response.Success,
					tc.wantSuccess,
				)
			}
		})
	}
}
