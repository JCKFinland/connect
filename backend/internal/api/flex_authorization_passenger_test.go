package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakePassengerAuthorizationExecutor struct {
	result      *flexauthorization.ExecuteResult
	err         error
	calls       int
	rideID      string
	auctionID   string
	passengerID string
}

func (f *fakePassengerAuthorizationExecutor) Execute(
	_ context.Context,
	rideID string,
	auctionID string,
	passengerID string,
) (*flexauthorization.ExecuteResult, error) {
	f.calls++
	f.rideID = rideID
	f.auctionID = auctionID
	f.passengerID = passengerID
	return f.result, f.err
}

func TestPassengerFlexAuthorizationHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	f, _, jwtService, customerID := passengerFlexJWTFixture(t)

	jwtToken := passengerFlexJWTToken(
		t,
		f,
		jwtService,
		customerID,
	)

	tests := []struct {
		name           string
		rideID         string
		auctionID      string
		token          string
		err            error
		result         *flexauthorization.ExecuteResult
		expectedStatus int
		expectedCalls  int
		expectSecret   bool
	}{
		{
			name:           "missing JWT",
			rideID:         f.RideID,
			auctionID:      f.AuctionID,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid ride identifier",
			rideID:         "invalid",
			auctionID:      f.AuctionID,
			token:          jwtToken,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid auction identifier",
			rideID:         f.RideID,
			auctionID:      "invalid",
			token:          jwtToken,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "ownership denied",
			rideID:         f.RideID,
			auctionID:      f.AuctionID,
			token:          jwtToken,
			err:            postgresrepo.ErrFlexAuthorizationAccessDenied,
			expectedStatus: http.StatusForbidden,
			expectedCalls:  1,
		},
		{
			name:           "authorization ineligible",
			rideID:         f.RideID,
			auctionID:      f.AuctionID,
			token:          jwtToken,
			err:            postgresrepo.ErrFlexAuthorizationNotEligible,
			expectedStatus: http.StatusConflict,
			expectedCalls:  1,
		},
		{
			name:           "pending reconciliation",
			rideID:         f.RideID,
			auctionID:      f.AuctionID,
			token:          jwtToken,
			err:            flexauthorization.ErrAuthorizationPending,
			expectedStatus: http.StatusConflict,
			expectedCalls:  1,
		},
		{
			name:           "service unavailable",
			rideID:         f.RideID,
			auctionID:      f.AuctionID,
			token:          jwtToken,
			err:            flexauthorization.ErrServiceUnavailable,
			expectedStatus: http.StatusServiceUnavailable,
			expectedCalls:  1,
		},
		{
			name:      "authorized",
			rideID:    f.RideID,
			auctionID: f.AuctionID,
			token:     jwtToken,
			result: &flexauthorization.ExecuteResult{
				AuthorizationID:         uuid.NewString(),
				Status:                  "AUTHORIZED",
				Authorized:              true,
				ProviderAuthorizationID: "pi_internal_only",
				ClientSecret:            "must_not_be_exposed",
			},
			expectedStatus: http.StatusOK,
			expectedCalls:  1,
		},
		{
			name:      "customer action required",
			rideID:    f.RideID,
			auctionID: f.AuctionID,
			token:     jwtToken,
			result: &flexauthorization.ExecuteResult{
				AuthorizationID:         uuid.NewString(),
				Status:                  "PROCESSING",
				RequiresCustomerAction:  true,
				ClientSecret:            "pi_test_secret_customer_action",
				ProviderAuthorizationID: "pi_internal_only",
			},
			expectedStatus: http.StatusOK,
			expectedCalls:  1,
			expectSecret:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executor := &fakePassengerAuthorizationExecutor{
				result: tc.result,
				err:    tc.err,
			}

			jwtAuth := middleware.NewAuthMiddleware(
				jwtService,
				repository.NewUserRepository(f.DB),
			)

			router := gin.New()

			group := router.Group("/api/v1/ride-requests")
			group.Use(jwtAuth.RequireAuth())

			handler := NewPassengerFlexAuthorizationHandler(
				executor,
			)

			group.POST(
				"/:id/flex-auctions/:auction_id/authorize",
				handler.Authorize,
			)

			path := "/api/v1/ride-requests/" +
				tc.rideID +
				"/flex-auctions/" +
				tc.auctionID +
				"/authorize"

			request := httptest.NewRequest(
				http.MethodPost,
				path,
				nil,
			)

			if tc.token != "" {
				request.Header.Set(
					"Authorization",
					"Bearer "+tc.token,
				)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.expectedStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tc.expectedStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if executor.calls != tc.expectedCalls {
				t.Fatalf(
					"expected %d executor calls, got %d",
					tc.expectedCalls,
					executor.calls,
				)
			}

			if tc.expectedCalls == 1 {
				if executor.rideID != tc.rideID ||
					executor.auctionID != tc.auctionID ||
					executor.passengerID != customerID {
					t.Fatalf(
						"authorization executor received incorrect identity",
					)
				}
			}

			var body map[string]interface{}
			if err := json.Unmarshal(
				recorder.Body.Bytes(),
				&body,
			); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if tc.expectedStatus != http.StatusOK {
				if body["success"] != false {
					t.Fatalf("expected unsuccessful response")
				}
				return
			}

			if body["success"] != true {
				t.Fatalf("expected successful response")
			}

			data, ok := body["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing authorization response data")
			}

			if _, exists := data["provider_authorization_id"]; exists {
				t.Fatal("provider authorization ID leaked")
			}

			if _, exists := data["idempotency_key"]; exists {
				t.Fatal("internal idempotency key leaked")
			}

			secret, exists := data["client_secret"]

			if tc.expectSecret {
				if !exists ||
					secret != tc.result.ClientSecret {
					t.Fatal("customer action secret missing")
				}
			} else if exists {
				t.Fatal("unexpected client secret exposure")
			}
		})
	}
}

func TestPassengerFlexAuthorizationNilExecutor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewPassengerFlexAuthorizationHandler(nil)
	router := gin.New()

	router.POST("/authorize", handler.Authorize)

	request := httptest.NewRequest(
		http.MethodPost,
		"/authorize",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected HTTP 503, got %d",
			recorder.Code,
		)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body["success"] != false {
		t.Fatalf("expected unsuccessful response")
	}
}
