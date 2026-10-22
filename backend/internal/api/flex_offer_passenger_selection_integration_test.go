package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newPassengerFlexSelectionRouter(
	f *testutil.FlexAcceptFixture,
) *gin.Engine {
	gin.SetMode(gin.TestMode)

	jwtService := security.NewJWTService(f.Config)
	userRepo := repository.NewUserRepository(f.DB)

	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		userRepo,
	)

	router := gin.New()

	rideRequests := router.Group("/api/v1/ride-requests")
	rideRequests.Use(authMiddleware.RequireAuth())

	handler := NewFlexOfferHandler(f.DB, f.Config)

	rideRequests.POST(
		"/:id/flex-auctions/:auction_id/offers/:offer_id/select",
		handler.SelectPassengerOffer,
	)

	return router
}

func executePassengerFlexSelectionRequest(
	router *gin.Engine,
	rideID, auctionID, offerID, token string,
) *httptest.ResponseRecorder {
	path := fmt.Sprintf(
		"/api/v1/ride-requests/%s/flex-auctions/%s/offers/%s/select",
		rideID, auctionID, offerID,
	)

	request := httptest.NewRequest(http.MethodPost, path, nil)

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func createPassengerFlexSelectionOffer(
	t *testing.T,
	f *testutil.FlexAcceptFixture,
) string {
	t.Helper()

	handler := NewFlexOfferHandler(f.DB, f.Config)

	offer, err := handler.service.SubmitCounterOffer(
		context.Background(),
		f.AuctionID,
		f.DriverID,
		2500,
	)
	if err != nil {
		t.Fatalf("create FLEX offer: %v", err)
	}

	return offer.ID
}

func TestPassengerFlexSelectionRejectsMissingJWT(t *testing.T) {
	f, _, _, _ := passengerFlexJWTFixture(t)

	offerID := createPassengerFlexSelectionOffer(t, f)
	router := newPassengerFlexSelectionRouter(f)

	recorder := executePassengerFlexSelectionRequest(
		router, f.RideID, f.AuctionID, offerID, "",
	)

	assertPassengerFlexJWTStatus(
		t, recorder, http.StatusUnauthorized,
	)
}

func TestPassengerFlexSelectionRejectsNonOwner(t *testing.T) {
	f, _, jwtService, _ := passengerFlexJWTFixture(t)

	offerID := createPassengerFlexSelectionOffer(t, f)
	router := newPassengerFlexSelectionRouter(f)

	token := passengerFlexJWTToken(
		t, f, jwtService, f.DriverID,
	)

	recorder := executePassengerFlexSelectionRequest(
		router, f.RideID, f.AuctionID, offerID, token,
	)

	assertPassengerFlexJWTStatus(
		t, recorder, http.StatusForbidden,
	)
}

func TestPassengerFlexSelectionRejectsInvalidIdentifiers(t *testing.T) {
	f, _, jwtService, customerID := passengerFlexJWTFixture(t)

	offerID := createPassengerFlexSelectionOffer(t, f)
	router := newPassengerFlexSelectionRouter(f)

	token := passengerFlexJWTToken(
		t, f, jwtService, customerID,
	)

	cases := []struct {
		name      string
		rideID    string
		auctionID string
		offerID   string
	}{
		{"ride", "invalid", f.AuctionID, offerID},
		{"auction", f.RideID, "invalid", offerID},
		{"offer", f.RideID, f.AuctionID, "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := executePassengerFlexSelectionRequest(
				router,
				tc.rideID,
				tc.auctionID,
				tc.offerID,
				token,
			)

			assertPassengerFlexJWTStatus(
				t, recorder, http.StatusBadRequest,
			)
		})
	}
}

func TestPassengerFlexSelectionSuccessAndReplayProtection(
	t *testing.T,
) {
	f, _, jwtService, customerID := passengerFlexJWTFixture(t)

	t.Cleanup(func() {
		var databaseName string

		err := f.DB.QueryRow(
			context.Background(),
			"SELECT current_database()",
		).Scan(&databaseName)
		if err != nil {
			t.Errorf("verify FLEX cleanup database: %v", err)
			return
		}

		if databaseName != "connect_settlement_test" {
			t.Errorf(
				"refusing selected-offer cleanup in database %q",
				databaseName,
			)
			return
		}

		_, err = f.DB.Exec(
			context.Background(),
			`UPDATE flex_auctions
			 SET status = 'OPEN',
			     selected_offer_id = NULL
			 WHERE id = $1
			   AND status = 'PENDING_AUTHORIZATION'`,
			f.AuctionID,
		)
		if err != nil {
			t.Errorf("clear selected FLEX offer reference: %v", err)
		}
	})

	offerID := createPassengerFlexSelectionOffer(t, f)
	router := newPassengerFlexSelectionRouter(f)

	token := passengerFlexJWTToken(
		t, f, jwtService, customerID,
	)

	recorder := executePassengerFlexSelectionRequest(
		router, f.RideID, f.AuctionID, offerID, token,
	)

	assertPassengerFlexJWTStatus(
		t, recorder, http.StatusOK,
	)

	var payload struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(), &payload,
	); err != nil {
		t.Fatalf("decode selection response: %v", err)
	}

	if !payload.Success {
		t.Fatalf(
			"successful selection marked failed: %s",
			recorder.Body.String(),
		)
	}

	allowed := map[string]bool{
		"id":                   true,
		"offered_amount_cents": true,
		"offer_type":           true,
		"status":               true,
		"offered_at":           true,
		"expires_at":           true,
	}

	for field := range payload.Data {
		if !allowed[field] {
			t.Fatalf("internal response field exposed: %s", field)
		}
	}

	for _, forbidden := range []string{
		"driver_id",
		"assignment_id",
		"vehicle_id",
		"company_id",
		"branch_id",
		"fleet_id",
	} {
		if strings.Contains(
			recorder.Body.String(),
			`"`+forbidden+`"`,
		) {
			t.Fatalf("internal field leaked: %s", forbidden)
		}
	}

	var auctionStatus string
	var selectedOfferID *string

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT status, selected_offer_id
         FROM flex_auctions WHERE id = $1`,
		f.AuctionID,
	).Scan(&auctionStatus, &selectedOfferID)

	if err != nil {
		t.Fatalf("verify selected auction: %v", err)
	}

	if auctionStatus != models.FlexAuctionStatusPendingAuthorization ||
		selectedOfferID == nil ||
		*selectedOfferID != offerID {
		t.Fatalf(
			"unexpected selected auction state: %s / %v",
			auctionStatus, selectedOfferID,
		)
	}

	var tripCount int
	err = f.DB.QueryRow(
		context.Background(),
		`SELECT COUNT(*) FROM trips WHERE ride_request_id = $1`,
		f.RideID,
	).Scan(&tripCount)

	if err != nil {
		t.Fatalf("count trips: %v", err)
	}

	if tripCount != 0 {
		t.Fatalf("selection created %d trips", tripCount)
	}

	var authorizationCount int
	err = f.DB.QueryRow(
		context.Background(),
		`SELECT COUNT(*) FROM flex_authorizations
         WHERE auction_id = $1`,
		f.AuctionID,
	).Scan(&authorizationCount)

	if err != nil {
		t.Fatalf("count authorizations: %v", err)
	}

	if authorizationCount != 0 {
		t.Fatalf(
			"selection created %d payment authorizations",
			authorizationCount,
		)
	}

	replay := executePassengerFlexSelectionRequest(
		router, f.RideID, f.AuctionID, offerID, token,
	)

	assertPassengerFlexJWTStatus(
		t, replay, http.StatusConflict,
	)
}

func TestPassengerFlexSelectionRejectsUnknownOffer(
	t *testing.T,
) {
	f, _, jwtService, customerID := passengerFlexJWTFixture(t)

	router := newPassengerFlexSelectionRouter(f)

	token := passengerFlexJWTToken(
		t, f, jwtService, customerID,
	)

	recorder := executePassengerFlexSelectionRequest(
		router,
		f.RideID,
		f.AuctionID,
		uuid.NewString(),
		token,
	)

	if recorder.Code != http.StatusNotFound &&
		recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected 404 or 409, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}
