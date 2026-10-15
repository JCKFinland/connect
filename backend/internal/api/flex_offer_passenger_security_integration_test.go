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
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newPassengerFlexJWTRouter(
	f *testutil.FlexAcceptFixture,
) (*gin.Engine, *security.JWTService) {
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

	rideRequests.GET(
		"/:id/flex-auctions/:auction_id/offers",
		handler.ListPassengerOffers,
	)

	return router, jwtService
}

func executePassengerFlexJWTRequest(
	router *gin.Engine,
	rideID string,
	auctionID string,
	token string,
) *httptest.ResponseRecorder {
	path := fmt.Sprintf(
		"/api/v1/ride-requests/%s/flex-auctions/%s/offers",
		rideID,
		auctionID,
	)

	request := httptest.NewRequest(http.MethodGet, path, nil)

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func passengerFlexJWTToken(
	t *testing.T,
	f *testutil.FlexAcceptFixture,
	jwtService *security.JWTService,
	userID string,
) string {
	t.Helper()

	user, err := repository.NewUserRepository(f.DB).GetByID(
		context.Background(),
		userID,
	)
	if err != nil {
		t.Fatalf("load authenticated user: %v", err)
	}

	if !user.IsActive {
		t.Fatal("JWT test user must be active")
	}

	token, err := jwtService.GenerateAccessToken(user)
	if err != nil {
		t.Fatalf("generate JWT: %v", err)
	}

	return token
}

func passengerFlexJWTFixture(t *testing.T) (
	*testutil.FlexAcceptFixture,
	*gin.Engine,
	*security.JWTService,
	string,
) {
	t.Helper()

	f := testutil.SetupFlexAcceptFixture(t)

	var customerID string

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT customer_id
		 FROM ride_requests
		 WHERE id = $1`,
		f.RideID,
	).Scan(&customerID)

	if err != nil {
		t.Fatalf("resolve ride passenger: %v", err)
	}

	router, jwtService := newPassengerFlexJWTRouter(f)

	return f, router, jwtService, customerID
}

func assertPassengerFlexJWTStatus(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	expected int,
) {
	t.Helper()

	if recorder.Code != expected {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			expected,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestPassengerFlexJWTSecurityRejectsMissingToken(t *testing.T) {
	f, router, _, _ := passengerFlexJWTFixture(t)

	recorder := executePassengerFlexJWTRequest(
		router,
		f.RideID,
		f.AuctionID,
		"",
	)

	assertPassengerFlexJWTStatus(
		t,
		recorder,
		http.StatusUnauthorized,
	)
}

func TestPassengerFlexJWTSecurityRejectsInvalidToken(t *testing.T) {
	f, router, _, _ := passengerFlexJWTFixture(t)

	recorder := executePassengerFlexJWTRequest(
		router,
		f.RideID,
		f.AuctionID,
		"invalid.jwt.token",
	)

	assertPassengerFlexJWTStatus(
		t,
		recorder,
		http.StatusUnauthorized,
	)
}

func TestPassengerFlexJWTSecurityOwnerSucceeds(t *testing.T) {
	f, router, jwtService, customerID := passengerFlexJWTFixture(t)

	handler := NewFlexOfferHandler(f.DB, f.Config)

	_, err := handler.service.SubmitCounterOffer(
		context.Background(),
		f.AuctionID,
		f.DriverID,
		2500,
	)
	if err != nil {
		t.Fatalf("create driver offer: %v", err)
	}

	token := passengerFlexJWTToken(
		t,
		f,
		jwtService,
		customerID,
	)

	recorder := executePassengerFlexJWTRequest(
		router,
		f.RideID,
		f.AuctionID,
		token,
	)

	assertPassengerFlexJWTStatus(
		t,
		recorder,
		http.StatusOK,
	)

	var payload struct {
		Success bool                         `json:"success"`
		Data    []map[string]json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !payload.Success || len(payload.Data) != 1 {
		t.Fatalf("unexpected passenger response: %s", recorder.Body.String())
	}

	allowed := map[string]bool{
		"id":                   true,
		"offered_amount_cents": true,
		"offer_type":           true,
		"status":               true,
		"offered_at":           true,
		"expires_at":           true,
	}

	for field := range payload.Data[0] {
		if !allowed[field] {
			t.Fatalf("unauthorized public field: %s", field)
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
}

func TestPassengerFlexJWTSecurityRejectsNonOwner(t *testing.T) {
	f, router, jwtService, _ := passengerFlexJWTFixture(t)

	token := passengerFlexJWTToken(
		t,
		f,
		jwtService,
		f.DriverID,
	)

	recorder := executePassengerFlexJWTRequest(
		router,
		f.RideID,
		f.AuctionID,
		token,
	)

	assertPassengerFlexJWTStatus(
		t,
		recorder,
		http.StatusForbidden,
	)
}

func TestPassengerFlexJWTSecurityRejectsUnknownAuction(t *testing.T) {
	f, router, jwtService, customerID := passengerFlexJWTFixture(t)

	token := passengerFlexJWTToken(
		t,
		f,
		jwtService,
		customerID,
	)

	recorder := executePassengerFlexJWTRequest(
		router,
		f.RideID,
		uuid.NewString(),
		token,
	)

	assertPassengerFlexJWTStatus(
		t,
		recorder,
		http.StatusNotFound,
	)
}

func TestPassengerFlexJWTSecurityRejectsExpiredAuction(t *testing.T) {
	f, router, jwtService, customerID := passengerFlexJWTFixture(t)

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

	token := passengerFlexJWTToken(
		t,
		f,
		jwtService,
		customerID,
	)

	recorder := executePassengerFlexJWTRequest(
		router,
		f.RideID,
		f.AuctionID,
		token,
	)

	assertPassengerFlexJWTStatus(
		t,
		recorder,
		http.StatusConflict,
	)
}
