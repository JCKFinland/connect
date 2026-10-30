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

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

func TestPaymentCreationErrorSecurityBoundary(t *testing.T) {
	const internalDetail = "postgres connection failed: secret-internal-database-host"

	tests := []struct {
		name        string
		serviceErr  error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "access denied",
			serviceErr:  payment.ErrPaymentAccessDenied,
			wantStatus:  http.StatusForbidden,
			wantMessage: "You are not authorized to create this payment",
		},
		{
			name:        "missing completed fare",
			serviceErr:  pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
			wantMessage: "Trip or completed fare not found",
		},
		{
			name:        "unexpected internal failure",
			serviceErr:  errors.New(internalDetail),
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/v1/trips/trip-security-test/payments",
				strings.NewReader(`{"payment_method":"CARD"}`),
			)

			c.Request.Header.Set(
				"Content-Type",
				"application/json",
			)

			c.Params = gin.Params{
				{
					Key:   "id",
					Value: "trip-security-test",
				},
			}

			user := setPaymentTestUser(c)
			called := false

			service := &fakePaymentService{
				createAuthorized: func(
					_ context.Context,
					tripID string,
					method string,
					userID string,
				) (*models.Payment, error) {
					called = true

					if tripID != "trip-security-test" {
						t.Errorf("unexpected trip ID: %q", tripID)
					}

					if method != "CARD" {
						t.Errorf("unexpected payment method: %q", method)
					}

					if userID != user.ID {
						t.Errorf("unexpected authenticated user ID: %q", userID)
					}

					return nil, tt.serviceErr
				},
			}

			NewPaymentHandler(service).CreateForCompletedTrip(c)

			if !called {
				t.Fatal("authorized payment service was not called")
			}

			if recorder.Code != tt.wantStatus {
				t.Fatalf(
					"HTTP status: got %d, want %d; body=%q",
					recorder.Code,
					tt.wantStatus,
					recorder.Body.String(),
				)
			}

			var body response.ErrorResponse

			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&body,
			); err != nil {
				t.Fatalf("decode error response: %v", err)
			}

			if body.Success {
				t.Fatal("expected success=false")
			}

			if body.Message != tt.wantMessage {
				t.Fatalf(
					"error message: got %q, want %q",
					body.Message,
					tt.wantMessage,
				)
			}

			if strings.Contains(
				recorder.Body.String(),
				internalDetail,
			) {
				t.Fatalf(
					"internal service error disclosed: %q",
					recorder.Body.String(),
				)
			}

			if strings.Contains(
				recorder.Body.String(),
				"secret-internal-database-host",
			) {
				t.Fatalf(
					"internal database host disclosed: %q",
					recorder.Body.String(),
				)
			}
		})
	}
}
