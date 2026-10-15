package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func passengerFlexHTTPFixture(t *testing.T) (
	*testutil.FlexAcceptFixture,
	*gin.Engine,
	string,
) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	f := testutil.SetupFlexAcceptFixture(t)

	var customerID string
	err := f.DB.QueryRow(
		context.Background(),
		`SELECT customer_id FROM ride_requests WHERE id = $1`,
		f.RideID,
	).Scan(&customerID)
	if err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	handler := NewFlexOfferHandler(f.DB, f.Config)

	router := gin.New()

	router.GET(
		"/ride-requests/:id/flex-auctions/:auction_id/offers",
		func(c *gin.Context) {
			identity := c.GetHeader("X-Test-User-ID")
			if identity != "" {
				middleware.SetCurrentUser(
					c,
					&models.User{
						BaseModel: models.BaseModel{
							ID: identity,
						},
					},
				)
			}
			c.Next()
		},
		handler.ListPassengerOffers,
	)

	return f, router, customerID
}

func passengerFlexHTTPGet(
	router *gin.Engine,
	rideID string,
	auctionID string,
	userID string,
) *httptest.ResponseRecorder {
	path := fmt.Sprintf(
		"/ride-requests/%s/flex-auctions/%s/offers",
		rideID,
		auctionID,
	)

	req := httptest.NewRequest(http.MethodGet, path, nil)

	if userID != "" {
		req.Header.Set("X-Test-User-ID", userID)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	return recorder
}

func TestPassengerFlexHTTPDiscoverySuccessAndPublicProjection(t *testing.T) {
	f, router, customerID := passengerFlexHTTPFixture(t)

	handler := NewFlexOfferHandler(f.DB, f.Config)

	_, err := handler.service.SubmitCounterOffer(
		context.Background(),
		f.AuctionID,
		f.DriverID,
		2500,
	)
	if err != nil {
		t.Fatalf("submit counter-offer: %v", err)
	}

	recorder := passengerFlexHTTPGet(
		router,
		f.RideID,
		f.AuctionID,
		customerID,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var payload struct {
		Success bool                         `json:"success"`
		Data    []map[string]json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !payload.Success || len(payload.Data) != 1 {
		t.Fatalf("unexpected discovery response: %s", recorder.Body.String())
	}

	offer := payload.Data[0]

	allowed := map[string]bool{
		"id":                   true,
		"offered_amount_cents": true,
		"offer_type":           true,
		"status":               true,
		"offered_at":           true,
		"expires_at":           true,
	}

	for field := range offer {
		if !allowed[field] {
			t.Fatalf("unexpected public field %q", field)
		}
	}

	var amount int64
	if err := json.Unmarshal(offer["offered_amount_cents"], &amount); err != nil {
		t.Fatalf("decode amount: %v", err)
	}
	if amount != 2500 {
		t.Fatalf("expected 2500 cents, got %d", amount)
	}

	for _, forbidden := range []string{
		"assignment_id",
		"driver_id",
		"vehicle_id",
		"company_id",
		"branch_id",
		"fleet_id",
	} {
		if strings.Contains(recorder.Body.String(), `"`+forbidden+`"`) {
			t.Fatalf("internal field leaked: %s", forbidden)
		}
	}
}

func TestPassengerFlexHTTPDiscoveryRejectsMissingIdentity(t *testing.T) {
	f, router, _ := passengerFlexHTTPFixture(t)

	recorder := passengerFlexHTTPGet(
		router,
		f.RideID,
		f.AuctionID,
		"",
	)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestPassengerFlexHTTPDiscoveryRejectsNonOwner(t *testing.T) {
	f, router, _ := passengerFlexHTTPFixture(t)

	recorder := passengerFlexHTTPGet(
		router,
		f.RideID,
		f.AuctionID,
		f.DriverID,
	)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected 403, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestPassengerFlexHTTPDiscoveryRejectsMalformedIDs(t *testing.T) {
	f, router, customerID := passengerFlexHTTPFixture(t)

	cases := []struct {
		name      string
		rideID    string
		auctionID string
	}{
		{
			name:      "invalid ride ID",
			rideID:    "invalid",
			auctionID: f.AuctionID,
		},
		{
			name:      "invalid auction ID",
			rideID:    f.RideID,
			auctionID: "invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := passengerFlexHTTPGet(
				router,
				tc.rideID,
				tc.auctionID,
				customerID,
			)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected 400, got %d: %s",
					recorder.Code,
					recorder.Body.String(),
				)
			}
		})
	}
}

func TestPassengerFlexHTTPDiscoveryRejectsUnknownAuction(t *testing.T) {
	f, router, customerID := passengerFlexHTTPFixture(t)

	recorder := passengerFlexHTTPGet(
		router,
		f.RideID,
		uuid.NewString(),
		customerID,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestPassengerFlexHTTPDiscoveryRejectsExpiredAuction(t *testing.T) {
	f, router, customerID := passengerFlexHTTPFixture(t)

	now := time.Now().UTC()

	_, err := f.DB.Exec(
		context.Background(),
		`UPDATE flex_auctions
		 SET opened_at = $2,
		     expires_at = $3
		 WHERE id = $1`,
		f.AuctionID,
		now.Add(-2*time.Minute),
		now.Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("expire auction: %v", err)
	}

	recorder := passengerFlexHTTPGet(
		router,
		f.RideID,
		f.AuctionID,
		customerID,
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected 409, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}
