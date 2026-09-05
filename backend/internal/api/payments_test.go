package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

func TestPaymentCreateRejectsMissingAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips/test-trip-id/payments",
		strings.NewReader(
			`{"payment_method":"CARD"}`,
		),
	)

	c.Request.Header.Set(
		"Content-Type",
		"application/json",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "test-trip-id",
		},
	}

	c.Set(
		"request_id",
		"payment-create-no-user",
	)

	handler := NewPaymentHandler(nil)

	handler.CreateForCompletedTrip(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}

	var body response.ErrorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if body.Success {
		t.Fatal(
			"expected success=false",
		)
	}

	if body.Message !=
		"Authenticated user not found" {

		t.Fatalf(
			"unexpected message: %q",
			body.Message,
		)
	}
}

func TestPaymentGetRejectsMissingAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/payments/test-payment-id",
		nil,
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "test-payment-id",
		},
	}

	c.Set(
		"request_id",
		"payment-get-no-user",
	)

	handler := NewPaymentHandler(nil)

	handler.GetPayment(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}

	var body response.ErrorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if body.Success {
		t.Fatal(
			"expected success=false",
		)
	}

	if body.Message !=
		"Authenticated user not found" {

		t.Fatalf(
			"unexpected message: %q",
			body.Message,
		)
	}
}

func TestTripPaymentGetRejectsMissingAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/test-trip-id/payment",
		nil,
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "test-trip-id",
		},
	}

	c.Set(
		"request_id",
		"trip-payment-get-no-user",
	)

	handler := NewPaymentHandler(nil)

	handler.GetTripPayment(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusUnauthorized,
			recorder.Code,
		)
	}

	var body response.ErrorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if body.Success {
		t.Fatal(
			"expected success=false",
		)
	}

	if body.Message !=
		"Authenticated user not found" {

		t.Fatalf(
			"unexpected message: %q",
			body.Message,
		)
	}
}

func setPaymentTestUser(
	c *gin.Context,
) *models.User {
	user := &models.User{}

	user.ID = "customer-test-user"

	middleware.SetCurrentUser(
		c,
		user,
	)

	return user
}

func TestPaymentCreateAccessDeniedReturnsForbidden(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips/trip-1/payments",
		strings.NewReader(
			`{"payment_method":"CARD"}`,
		),
	)

	c.Request.Header.Set(
		"Content-Type",
		"application/json",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "trip-1",
		},
	}

	setPaymentTestUser(c)

	handler := NewPaymentHandler(
		&fakePaymentService{
			createAuthorized: func(
				context.Context,
				string,
				string,
				string,
			) (*models.Payment, error) {
				return nil, payment.ErrPaymentAccessDenied
			},
		},
	)

	handler.CreateForCompletedTrip(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusForbidden,
			recorder.Code,
		)
	}
}

func TestPaymentCreateSuccessReturnsCreated(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips/trip-1/payments",
		strings.NewReader(
			`{"payment_method":"CARD"}`,
		),
	)

	c.Request.Header.Set(
		"Content-Type",
		"application/json",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "trip-1",
		},
	}

	user := setPaymentTestUser(c)

	handler := NewPaymentHandler(
		&fakePaymentService{
			createAuthorized: func(
				_ context.Context,
				tripID string,
				method string,
				userID string,
			) (*models.Payment, error) {
				if tripID != "trip-1" {
					t.Fatalf(
						"unexpected trip ID %q",
						tripID,
					)
				}

				if method != "CARD" {
					t.Fatalf(
						"unexpected payment method %q",
						method,
					)
				}

				if userID != user.ID {
					t.Fatalf(
						"unexpected user ID %q",
						userID,
					)
				}

				return &models.Payment{
					TripID:        tripID,
					CustomerID:    userID,
					PaymentMethod: method,
					Status:        "PENDING",
					Amount:        "10.00",
					Currency:      "EUR",
				}, nil
			},
		},
	)

	handler.CreateForCompletedTrip(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusCreated,
			recorder.Code,
		)
	}
}

func TestPaymentGetAccessDeniedAndNotFound(
	t *testing.T,
) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
	}{
		{
			name:       "access denied",
			serviceErr: payment.ErrPaymentAccessDenied,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "not found",
			serviceErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)

				c.Request = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/payments/payment-1",
					nil,
				)

				c.Params = gin.Params{
					{
						Key:   "id",
						Value: "payment-1",
					},
				}

				setPaymentTestUser(c)

				handler := NewPaymentHandler(
					&fakePaymentService{
						getByIDAuthorized: func(
							context.Context,
							string,
							string,
						) (*models.Payment, error) {
							return nil, tt.serviceErr
						},
					},
				)

				handler.GetPayment(c)

				if recorder.Code != tt.wantStatus {
					t.Fatalf(
						"expected %d, got %d",
						tt.wantStatus,
						recorder.Code,
					)
				}
			},
		)
	}
}

func TestTripPaymentGetSuccessReturnsOK(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/trip-1/payment",
		nil,
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "trip-1",
		},
	}

	user := setPaymentTestUser(c)

	handler := NewPaymentHandler(
		&fakePaymentService{
			getByTripIDAuthorized: func(
				_ context.Context,
				tripID string,
				userID string,
			) (*models.Payment, error) {
				if tripID != "trip-1" {
					t.Fatalf(
						"unexpected trip ID %q",
						tripID,
					)
				}

				if userID != user.ID {
					t.Fatalf(
						"unexpected user ID %q",
						userID,
					)
				}

				return &models.Payment{
					TripID:     tripID,
					CustomerID: userID,
					Status:     "PENDING",
					Amount:     "10.00",
					Currency:   "EUR",
				}, nil
			},
		},
	)

	handler.GetTripPayment(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected %d, got %d",
			http.StatusOK,
			recorder.Code,
		)
	}
}
