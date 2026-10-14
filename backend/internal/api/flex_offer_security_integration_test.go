package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
	"github.com/JCKFinland/connect/backend/internal/services/rbac"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newFlexOfferSecurityIntegrationRouter(
	f *testutil.FlexAcceptFixture,
) (*gin.Engine, *security.JWTService) {
	gin.SetMode(gin.TestMode)

	jwtService := security.NewJWTService(f.Config)

	userRepo := repository.NewUserRepository(f.DB)
	permissionRepo := repository.NewPermissionRepository(f.DB)

	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		userRepo,
	)

	rbacMiddleware := middleware.NewRBACMiddleware(
		rbac.NewService(permissionRepo),
	)

	router := gin.New()

	drivers := router.Group("/api/v1/drivers")
	drivers.Use(authMiddleware.RequireAuth())

	handler := NewFlexOfferHandler(f.DB, f.Config)

	drivers.POST(
		"/flex-auctions/:auction_id/offers/accept",
		rbacMiddleware.RequirePermission("driver.operations"),
		handler.SubmitAcceptFare,
	)

	return router, jwtService
}

func executeFlexOfferSecurityRequest(
	router *gin.Engine,
	auctionID string,
	token string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/drivers/flex-auctions/"+
			auctionID+"/offers/accept",
		nil,
	)

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestFlexOfferSecurityIntegrationRejectsMissingToken(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router, _ := newFlexOfferSecurityIntegrationRouter(f)

	recorder := executeFlexOfferSecurityRequest(
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

func TestFlexOfferSecurityIntegrationRejectsInvalidJWT(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router, _ := newFlexOfferSecurityIntegrationRouter(f)

	recorder := executeFlexOfferSecurityRequest(
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

func TestFlexOfferSecurityIntegrationRejectsUserWithoutPermission(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	customerID := uuid.NewString()

	_, err := f.DB.Exec(
		ctx,
		`INSERT INTO users (
			id, email, password_hash,
			first_name, last_name, phone,
			is_active, is_verified
		)
		VALUES (
			$1, $2, $3,
			'FLEX', 'Unauthorized', '0000000000',
			TRUE, TRUE
		)`,
		customerID,
		"flex-rbac-"+customerID+"@connect.test",
		"test-only-password-hash",
	)
	if err != nil {
		t.Fatalf("create disposable RBAC test user: %v", err)
	}

	t.Cleanup(func() {
		_, cleanupErr := f.DB.Exec(
			context.Background(),
			`DELETE FROM users WHERE id = $1`,
			customerID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup disposable RBAC user: %v", cleanupErr)
		}
	})

	// Verify the disposable user is readable through the same
	// repository used by production authentication middleware.
	userRepo := repository.NewUserRepository(f.DB)

	storedUser, lookupErr := userRepo.GetByID(ctx, customerID)
	if lookupErr != nil {
		t.Fatalf(
			"disposable RBAC user not readable through repository: %v",
			lookupErr,
		)
	}

	if storedUser == nil || storedUser.ID != customerID {
		t.Fatalf(
			"unexpected disposable RBAC user: %+v",
			storedUser,
		)
	}

	router, jwtService := newFlexOfferSecurityIntegrationRouter(f)

	user := &models.User{
		BaseModel: models.BaseModel{ID: customerID},
		Email:     "flex-rbac-" + customerID + "@connect.test",
		IsActive:  true,
	}

	token, err := jwtService.GenerateAccessToken(user)
	if err != nil {
		t.Fatalf("generate unauthorized-user JWT: %v", err)
	}

	recorder := executeFlexOfferSecurityRequest(
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

func TestFlexOfferSecurityIntegrationAuthorizedDriverSucceeds(
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
		t.Fatalf("generate driver JWT: %v", err)
	}

	recorder := executeFlexOfferSecurityRequest(
		router,
		f.AuctionID,
		token,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected authorized driver HTTP 201, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var persistedDriverID string
	var count int

	err = f.DB.QueryRow(
		context.Background(),
		`SELECT driver_id, COUNT(*) OVER ()
		 FROM flex_offers
		 WHERE auction_id = $1`,
		f.AuctionID,
	).Scan(&persistedDriverID, &count)

	if err != nil {
		t.Fatalf("verify authenticated driver offer: %v", err)
	}

	if count != 1 || persistedDriverID != f.DriverID {
		t.Fatalf(
			"unexpected persisted offer identity: count=%d driver=%s expected=%s",
			count,
			persistedDriverID,
			f.DriverID,
		)
	}
}
