package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

type countingStripeProvisioningService struct {
	calls int
}

func (s *countingStripeProvisioningService) ProvisionAccount(
	_ context.Context,
	_, _ string,
) (string, error) {
	s.calls++
	return "acct_1234567890abcdef", nil
}

func TestStripeProvisioningRealJWTAndRBACRejectUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixture := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	actorID := uuid.NewString()
	actorEmail := "stripe-provision-rbac-" + actorID + "@connect.test"

	_, err := fixture.DB.Exec(
		ctx,
		`INSERT INTO users (
            id, email, password_hash,
            first_name, last_name, phone,
            is_active, is_verified
        )
        VALUES (
            $1, $2, 'test-only-password-hash',
            'Stripe', 'Unauthorized', '0000000000',
            TRUE, TRUE
        )`,
		actorID,
		actorEmail,
	)
	if err != nil {
		t.Fatalf("create isolated RBAC actor: %v", err)
	}

	t.Cleanup(func() {
		_, cleanupErr := fixture.DB.Exec(
			context.Background(),
			`DELETE FROM users WHERE id = $1`,
			actorID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup isolated RBAC actor: %v", cleanupErr)
		}
	})

	userRepo := repository.NewUserRepository(fixture.DB)
	stored, err := userRepo.GetByID(ctx, actorID)
	if err != nil || stored == nil || stored.ID != actorID {
		t.Fatalf("test actor not readable through auth repository: %v", err)
	}

	jwtService := security.NewJWTService(fixture.Config)
	actor := &models.User{
		BaseModel: models.BaseModel{ID: actorID},
		Email:     actorEmail,
		IsActive:  true,
	}
	validToken, err := jwtService.GenerateAccessToken(actor)
	if err != nil {
		t.Fatalf("generate isolated actor JWT: %v", err)
	}

	auth := middleware.NewAuthMiddleware(jwtService, userRepo)
	permissions := repository.NewPermissionRepository(fixture.DB)
	rbacMiddleware := middleware.NewRBACMiddleware(
		rbac.NewService(permissions),
	)

	service := &countingStripeProvisioningService{}
	router := gin.New()

	if !RegisterStripeProvisioningRoutes(
		router,
		auth,
		rbacMiddleware,
		NewStripeProvisioningHandler(service),
	) {
		t.Fatal("complete dependencies did not register protected route")
	}

	const route = "/api/v1/drivers/"
	path := route + uuid.NewString() + "/stripe-provisioning"

	cases := []struct {
		name   string
		token  string
		status int
	}{
		{
			name:   "missing_jwt",
			status: http.StatusUnauthorized,
		},
		{
			name:   "invalid_jwt",
			token:  "invalid.jwt.token",
			status: http.StatusUnauthorized,
		},
		{
			name:   "authenticated_without_drivers_manage",
			token:  validToken,
			status: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.status {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tc.status,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			if service.calls != 0 {
				t.Fatalf(
					"unauthorized request reached provisioning service: %d calls",
					service.calls,
				)
			}
		})
	}

	t.Run("inactive_authenticated_user", func(t *testing.T) {
		_, updateErr := fixture.DB.Exec(
			ctx,
			`UPDATE users SET is_active = FALSE WHERE id = $1`,
			actorID,
		)
		if updateErr != nil {
			t.Fatalf("disable isolated actor: %v", updateErr)
		}

		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf(
				"expected HTTP 403 for inactive user, got %d: %s",
				recorder.Code,
				recorder.Body.String(),
			)
		}

		if service.calls != 0 {
			t.Fatalf(
				"inactive user reached provisioning service: %d calls",
				service.calls,
			)
		}
	})
}

// authorizedProvisioningPermissions replaces only the permission storage
// boundary. The real RBAC service and middleware remain under test.
type authorizedProvisioningPermissions struct {
	repository.PermissionRepository
	actorID    string
	permission string
	calls      int
}

func (p *authorizedProvisioningPermissions) HasPermission(
	_ context.Context,
	actorID string,
	permission string,
) (bool, error) {
	p.calls++
	p.actorID = actorID
	p.permission = permission
	return permission == "drivers.manage", nil
}

type identityCapturingProvisioningService struct {
	calls    int
	actorID  string
	driverID string
}

func (s *identityCapturingProvisioningService) ProvisionAccount(
	_ context.Context,
	actorID string,
	driverID string,
) (string, error) {
	s.calls++
	s.actorID = actorID
	s.driverID = driverID
	return "acct_1234567890abcdef", nil
}

func TestStripeProvisioningAuthorizedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixture := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	actorID := uuid.NewString()
	driverID := uuid.NewString()
	actorEmail := "stripe-provision-authorized-" +
		actorID + "@connect.test"

	_, err := fixture.DB.Exec(
		ctx,
		`INSERT INTO users (
            id, email, password_hash,
            first_name, last_name, phone,
            is_active, is_verified
        ) VALUES (
            $1, $2, 'test-only-password-hash',
            'Stripe', 'Authorized', '0000000000',
            TRUE, TRUE
        )`,
		actorID,
		actorEmail,
	)
	if err != nil {
		t.Fatalf("create authorized test actor: %v", err)
	}

	t.Cleanup(func() {
		_, cleanupErr := fixture.DB.Exec(
			context.Background(),
			`DELETE FROM users WHERE id = $1`,
			actorID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup authorized test actor: %v", cleanupErr)
		}
	})

	jwtService := security.NewJWTService(fixture.Config)
	token, err := jwtService.GenerateAccessToken(&models.User{
		BaseModel: models.BaseModel{ID: actorID},
		Email:     actorEmail,
		IsActive:  true,
	})
	if err != nil {
		t.Fatalf("generate authorized actor JWT: %v", err)
	}

	permissions := &authorizedProvisioningPermissions{}
	service := &identityCapturingProvisioningService{}

	router := gin.New()

	registered := RegisterStripeProvisioningRoutes(
		router,
		middleware.NewAuthMiddleware(
			jwtService,
			repository.NewUserRepository(fixture.DB),
		),
		middleware.NewRBACMiddleware(
			rbac.NewService(permissions),
		),
		NewStripeProvisioningHandler(service),
	)
	if !registered {
		t.Fatal("authorized provisioning route not registered")
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/drivers/"+driverID+"/stripe-provisioning",
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+token)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if permissions.calls != 1 ||
		permissions.actorID != actorID ||
		permissions.permission != "drivers.manage" {
		t.Fatalf(
			"incorrect RBAC check: calls=%d actor=%q permission=%q",
			permissions.calls,
			permissions.actorID,
			permissions.permission,
		)
	}

	if service.calls != 1 ||
		service.actorID != actorID ||
		service.driverID != driverID {
		t.Fatalf(
			"incorrect provisioning identity: calls=%d actor=%q driver=%q",
			service.calls,
			service.actorID,
			service.driverID,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"payouts_enabled":false`,
	) {
		t.Fatalf(
			"payouts safety flag missing: %s",
			recorder.Body.String(),
		)
	}
}
