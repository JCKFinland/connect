package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	flexoffer "github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestPassengerFlexConfirmationJWTBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixture, _, jwtService, passengerID :=
		passengerFlexJWTFixture(t)

	validToken := passengerFlexJWTToken(
		t,
		fixture,
		jwtService,
		passengerID,
	)

	cases := []struct {
		name       string
		token      string
		rideID     string
		auctionID  string
		serviceErr error
		wantStatus int
		wantCalls  int
	}{
		{
			name:       "missing JWT",
			rideID:     fixture.RideID,
			auctionID:  fixture.AuctionID,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid JWT",
			token:      "invalid.jwt.token",
			rideID:     fixture.RideID,
			auctionID:  fixture.AuctionID,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "authenticated passenger",
			token:      validToken,
			rideID:     fixture.RideID,
			auctionID:  fixture.AuctionID,
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "invalid ride identifier",
			token:      validToken,
			rideID:     "invalid",
			auctionID:  fixture.AuctionID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid auction identifier",
			token:      validToken,
			rideID:     fixture.RideID,
			auctionID:  "invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "passenger ownership denied",
			token:      validToken,
			rideID:     fixture.RideID,
			auctionID:  fixture.AuctionID,
			serviceErr: flexoffer.ErrFlexConfirmationAccessDenied,
			wantStatus: http.StatusForbidden,
			wantCalls:  1,
		},
		{
			name:       "confirmation not eligible",
			token:      validToken,
			rideID:     fixture.RideID,
			auctionID:  fixture.AuctionID,
			serviceErr: flexoffer.ErrFlexConfirmationNotEligible,
			wantStatus: http.StatusConflict,
			wantCalls:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			confirmer := &fakePassengerFlexRideConfirmer{
				err: tc.serviceErr,
				trip: &models.Trip{
					BaseModel: models.BaseModel{
						ID: uuid.NewString(),
					},
					RideRequestID: tc.rideID,
					Status:        "ASSIGNED",
				},
			}

			auth := middleware.NewAuthMiddleware(
				jwtService,
				repository.NewUserRepository(fixture.DB),
			)

			router := gin.New()

			group := router.Group("/api/v1/ride-requests")
			group.Use(auth.RequireAuth())

			handler := NewPassengerFlexConfirmationHandler(
				confirmer,
			)

			group.POST(
				"/:id/flex-auctions/:auction_id/confirm",
				handler.Confirm,
			)

			path := "/api/v1/ride-requests/" +
				tc.rideID +
				"/flex-auctions/" +
				tc.auctionID +
				"/confirm"

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

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tc.wantStatus,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if confirmer.calls != tc.wantCalls {
				t.Fatalf(
					"expected %d confirmation calls, got %d",
					tc.wantCalls,
					confirmer.calls,
				)
			}

			if tc.wantCalls == 1 {
				if confirmer.rideID != tc.rideID ||
					confirmer.auctionID != tc.auctionID ||
					confirmer.passengerID != passengerID {
					t.Fatal(
						"confirmation service received incorrect authenticated identity",
					)
				}
			}
		})
	}
}
