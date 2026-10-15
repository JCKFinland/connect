package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func executeFlexCounterOfferSecurityRequest(
	router *gin.Engine,
	auctionID string,
	token string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/drivers/flex-auctions/"+
			auctionID+"/offers/counter",
		strings.NewReader(`{"proposed_amount_cents":2500}`),
	)

	request.Header.Set("Content-Type", "application/json")

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestFlexCounterOfferSecurityRejectsMissingJWT(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router, _ := newFlexOfferSecurityIntegrationRouter(f)

	recorder := executeFlexCounterOfferSecurityRequest(
		router,
		f.AuctionID,
		"",
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusUnauthorized,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexCounterOfferSecurityRejectsInvalidJWT(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router, _ := newFlexOfferSecurityIntegrationRouter(f)

	recorder := executeFlexCounterOfferSecurityRequest(
		router,
		f.AuctionID,
		"invalid.jwt.token",
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusUnauthorized,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexCounterOfferSecurityRejectsUserWithoutPermission(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	customerID := uuid.NewString()
	email := "flex-counter-rbac-" + customerID + "@connect.test"

	_, err := f.DB.Exec(
		ctx,
		`INSERT INTO users (
            id, email, password_hash,
            first_name, last_name, phone,
            is_active, is_verified
        )
        VALUES (
            $1, $2, 'test-only-password-hash',
            'FLEX', 'Unauthorized', '0000000000',
            TRUE, TRUE
        )`,
		customerID,
		email,
	)
	if err != nil {
		t.Fatalf("create unauthorized test user: %v", err)
	}

	t.Cleanup(func() {
		_, cleanupErr := f.DB.Exec(
			context.Background(),
			`DELETE FROM users WHERE id = $1`,
			customerID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup unauthorized test user: %v", cleanupErr)
		}
	})

	router, jwtService := newFlexOfferSecurityIntegrationRouter(f)

	user := &models.User{
		BaseModel: models.BaseModel{ID: customerID},
		Email:     email,
		IsActive:  true,
	}

	token, err := jwtService.GenerateAccessToken(user)
	if err != nil {
		t.Fatalf("generate unauthorized user JWT: %v", err)
	}

	recorder := executeFlexCounterOfferSecurityRequest(
		router,
		f.AuctionID,
		token,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusForbidden,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexCounterOfferSecurityAuthorizedDriverSucceeds(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)

	router, jwtService := newFlexOfferSecurityIntegrationRouter(f)

	userRepo := repository.NewUserRepository(f.DB)

	driverUser, err := userRepo.GetByID(
		context.Background(),
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("load fixture driver user: %v", err)
	}

	token, err := jwtService.GenerateAccessToken(driverUser)
	if err != nil {
		t.Fatalf("generate authorized driver JWT: %v", err)
	}

	recorder := executeFlexCounterOfferSecurityRequest(
		router,
		f.AuctionID,
		token,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected HTTP 201, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var count int
	var driverID string
	var offerType string
	var amount string

	err = f.DB.QueryRow(
		context.Background(),
		`SELECT driver_id, offer_type,
                offered_amount::text,
                COUNT(*) OVER ()
         FROM flex_offers
         WHERE auction_id = $1`,
		f.AuctionID,
	).Scan(
		&driverID,
		&offerType,
		&amount,
		&count,
	)
	if err != nil {
		t.Fatalf("verify authenticated counter-offer: %v", err)
	}

	if count != 1 ||
		driverID != f.DriverID ||
		offerType != models.FlexOfferTypeCounterOffer ||
		amount != "25.00" {
		t.Fatalf(
			"unexpected persisted counter-offer: count=%d driver=%s type=%s amount=%s",
			count,
			driverID,
			offerType,
			amount,
		)
	}
}
