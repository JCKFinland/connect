package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	flexoffer "github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakePassengerFlexRideConfirmer struct {
	trip        *models.Trip
	err         error
	calls       int
	rideID      string
	auctionID   string
	passengerID string
}

func (f *fakePassengerFlexRideConfirmer) ConfirmAuthorizedRide(
	_ context.Context,
	rideID string,
	auctionID string,
	passengerID string,
) (*models.Trip, error) {
	f.calls++
	f.rideID = rideID
	f.auctionID = auctionID
	f.passengerID = passengerID
	return f.trip, f.err
}

func TestPassengerFlexConfirmationHandlerContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rideID := uuid.NewString()
	auctionID := uuid.NewString()
	passengerID := uuid.NewString()

	cases := []struct {
		name          string
		rideID        string
		auctionID     string
		confirmer     *fakePassengerFlexRideConfirmer
		authenticated bool
		wantStatus    int
		wantCalls     int
	}{
		{
			name:          "success",
			rideID:        rideID,
			auctionID:     auctionID,
			authenticated: true,
			confirmer: &fakePassengerFlexRideConfirmer{
				trip: &models.Trip{
					BaseModel:     models.BaseModel{ID: uuid.NewString()},
					RideRequestID: rideID,
					Status:        "ASSIGNED",
				},
			},
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
		{
			name:       "missing identity",
			rideID:     rideID,
			auctionID:  auctionID,
			confirmer:  &fakePassengerFlexRideConfirmer{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "invalid ride ID",
			rideID:        "invalid",
			auctionID:     auctionID,
			authenticated: true,
			confirmer:     &fakePassengerFlexRideConfirmer{},
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "invalid auction ID",
			rideID:        rideID,
			auctionID:     "invalid",
			authenticated: true,
			confirmer:     &fakePassengerFlexRideConfirmer{},
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "access denied",
			rideID:        rideID,
			auctionID:     auctionID,
			authenticated: true,
			confirmer: &fakePassengerFlexRideConfirmer{
				err: flexoffer.ErrFlexConfirmationAccessDenied,
			},
			wantStatus: http.StatusForbidden,
			wantCalls:  1,
		},
		{
			name:          "not eligible",
			rideID:        rideID,
			auctionID:     auctionID,
			authenticated: true,
			confirmer: &fakePassengerFlexRideConfirmer{
				err: flexoffer.ErrFlexConfirmationNotEligible,
			},
			wantStatus: http.StatusConflict,
			wantCalls:  1,
		},
		{
			name:          "internal failure",
			rideID:        rideID,
			auctionID:     auctionID,
			authenticated: true,
			confirmer: &fakePassengerFlexRideConfirmer{
				err: errors.New("private database failure"),
			},
			wantStatus: http.StatusInternalServerError,
			wantCalls:  1,
		},
		{
			name:          "nil trip",
			rideID:        rideID,
			auctionID:     auctionID,
			authenticated: true,
			confirmer:     &fakePassengerFlexRideConfirmer{},
			wantStatus:    http.StatusInternalServerError,
			wantCalls:     1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()

			router.POST(
				"/ride-requests/:id/flex-auctions/:auction_id/confirm",
				func(c *gin.Context) {
					if tc.authenticated {
						c.Set("user", &models.User{
							BaseModel: models.BaseModel{
								ID: passengerID,
							},
						})
					}
					NewPassengerFlexConfirmationHandler(
						tc.confirmer,
					).Confirm(c)
				},
			)

			path := "/ride-requests/" + tc.rideID +
				"/flex-auctions/" + tc.auctionID + "/confirm"

			request := httptest.NewRequest(
				http.MethodPost,
				path,
				nil,
			)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"status=%d want=%d body=%s",
					recorder.Code,
					tc.wantStatus,
					recorder.Body.String(),
				)
			}

			if tc.confirmer.calls != tc.wantCalls {
				t.Fatalf(
					"calls=%d want=%d",
					tc.confirmer.calls,
					tc.wantCalls,
				)
			}

			if tc.wantCalls == 1 &&
				tc.confirmer.passengerID != passengerID {
				t.Fatalf("wrong authenticated passenger identity")
			}

			if tc.wantStatus == http.StatusOK {
				var body map[string]any
				if err := json.Unmarshal(
					recorder.Body.Bytes(),
					&body,
				); err != nil {
					t.Fatal(err)
				}

				data, ok := body["data"].(map[string]any)
				if !ok {
					t.Fatalf("missing public trip data: %s", recorder.Body.String())
				}

				if data["ride_request_id"] != rideID ||
					data["status"] != "ASSIGNED" {
					t.Fatalf("unexpected trip response: %+v", data)
				}

				if _, exposed := data["driver_id"]; exposed {
					t.Fatal("internal driver ID exposed")
				}
			}
		})
	}
}
