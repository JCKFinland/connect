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

type tripCompleteHTTPTestService struct {
	trip.Service

	completeErr error
	fare        *models.TripFare

	completeCalls int
	tripID        string
	userID        string
}

func (s *tripCompleteHTTPTestService) CompleteTrip(
	_ context.Context,
	tripID string,
	userID string,
) (*models.TripFare, error) {
	s.completeCalls++
	s.tripID = tripID
	s.userID = userID

	if s.completeErr != nil {
		return nil, s.completeErr
	}

	return s.fare, nil
}

func TestTripCompleteHTTPAuthorizationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tripID = "trip-complete-http-123"
		userID = "driver-complete-http-456"
		secret = "private-completion-database-and-settlement-details"
	)

	cases := []struct {
		name          string
		authenticated bool
		completeErr   error
		wantStatus    int
		wantCalls     int
		wantSuccess   bool
		wantMessage   string
		wantFare      bool
	}{
		{
			name:        "unauthenticated request",
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "authenticated user not found",
		},
		{
			name:          "trip not in progress",
			authenticated: true,
			completeErr: fmt.Errorf(
				"%w: trip must be IN_PROGRESS",
				trip.ErrTripCompletionValidation,
			),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   1,
			wantMessage: "Trip is not ready for completion",
		},
		{
			name:          "insufficient GPS evidence",
			authenticated: true,
			completeErr: fmt.Errorf(
				"%w: insufficient trip location evidence",
				trip.ErrTripCompletionValidation,
			),
			wantStatus:  http.StatusBadRequest,
			wantCalls:   1,
			wantMessage: "Trip is not ready for completion",
		},
		{
			name:          "unauthorized completion",
			authenticated: true,
			completeErr:   trip.ErrTripStatusAccessDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
			wantMessage:   "You are not authorized to complete this trip",
		},
		{
			name:          "wrapped unauthorized completion",
			authenticated: true,
			completeErr: fmt.Errorf(
				"completion authorization: %w",
				trip.ErrTripStatusAccessDenied,
			),
			wantStatus:  http.StatusForbidden,
			wantCalls:   1,
			wantMessage: "You are not authorized to complete this trip",
		},
		{
			name:          "missing trip",
			authenticated: true,
			completeErr:   pgx.ErrNoRows,
			wantStatus:    http.StatusNotFound,
			wantCalls:     1,
			wantMessage:   "Trip not found",
		},
		{
			name:          "wrapped missing trip",
			authenticated: true,
			completeErr: fmt.Errorf(
				"load trip for completion: %w",
				pgx.ErrNoRows,
			),
			wantStatus:  http.StatusNotFound,
			wantCalls:   1,
			wantMessage: "Trip not found",
		},
		{
			name:          "internal fare persistence failure",
			authenticated: true,
			completeErr:   errors.New(secret),
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
			wantMessage:   "Failed to complete trip",
		},
		{
			name:          "wrapped settlement failure",
			authenticated: true,
			completeErr: fmt.Errorf(
				"FLEX settlement failed: %w",
				errors.New(secret),
			),
			wantStatus:  http.StatusInternalServerError,
			wantCalls:   1,
			wantMessage: "Failed to complete trip",
		},
		{
			name:          "successful completion",
			authenticated: true,
			wantStatus:    http.StatusOK,
			wantCalls:     1,
			wantSuccess:   true,
			wantMessage:   "Trip completed successfully",
			wantFare:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &tripCompleteHTTPTestService{
				completeErr: tc.completeErr,
				fare:        &models.TripFare{},
			}

			router := gin.New()

			router.POST(
				"/api/v1/trips/:id/complete",
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
				NewTripHandler(service).Complete,
			)

			recorder := httptest.NewRecorder()

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/trips/"+tripID+"/complete",
				nil,
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

			if service.completeCalls != tc.wantCalls {
				t.Fatalf(
					"completion calls = %d, want %d",
					service.completeCalls,
					tc.wantCalls,
				)
			}

			if strings.Contains(recorder.Body.String(), secret) {
				t.Fatalf(
					"internal completion details disclosed: %s",
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

			if tc.wantFare {
				if len(response.Data) == 0 ||
					string(response.Data) == "null" {
					t.Fatal("successful completion did not return fare")
				}
			} else if len(response.Data) != 0 &&
				string(response.Data) != "null" {
				t.Fatalf(
					"unexpected completion fare: %s",
					response.Data,
				)
			}

			if tc.wantCalls == 0 {
				if service.tripID != "" || service.userID != "" {
					t.Fatal("unexpected completion service arguments")
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
		})
	}
}
