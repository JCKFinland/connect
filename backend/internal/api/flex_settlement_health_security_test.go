package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
	"github.com/JCKFinland/connect/backend/internal/services/rbac"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const settlementHealthTestPath = "/api/v1/admin/flex-settlements/health"

func settlementHealthSecurityDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated settlement integration database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := pgxpool.New(
		ctx,
		"host=localhost port=5432 user=postgres dbname=connect_settlement_test sslmode=disable",
	)
	if err != nil {
		t.Fatalf("create settlement test database pool: %v", err)
	}

	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping settlement test database: %v", err)
	}

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("read database identity: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database identity: %q", databaseName)
	}

	return db
}

func settlementHealthSecurityRouter(
	db *pgxpool.Pool,
) (*gin.Engine, *security.JWTService) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.JWT.Secret = "settlement-health-integration-secret-at-least-32-bytes"
	cfg.JWT.Issuer = "connect-settlement-health-test"
	cfg.JWT.AccessTokenDuration = time.Hour

	jwtService := security.NewJWTService(cfg)

	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		repository.NewUserRepository(db),
	)

	rbacMiddleware := middleware.NewRBACMiddleware(
		rbac.NewService(repository.NewPermissionRepository(db)),
	)

	router := gin.New()
	v1 := router.Group("/api/v1")

	// Mirror the production route's independent admin group.
	admin := v1.Group("/admin")
	admin.Use(authMiddleware.RequireAuth())
	admin.Use(rbacMiddleware.RequirePermission("users.read"))
	admin.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	settlementAdmin := v1.Group("/admin")
	settlementAdmin.Use(authMiddleware.RequireAuth())
	settlementAdmin.Use(
		rbacMiddleware.RequirePermission("flex_settlements.health.read"),
	)
	settlementAdmin.GET(
		"/flex-settlements/health",
		FlexSettlementHealthHandler(db),
	)

	return router, jwtService
}

func settlementHealthSecurityUser(
	t *testing.T,
	db *pgxpool.Pool,
	roleName string,
) string {
	t.Helper()

	ctx := context.Background()
	userID := uuid.NewString()

	_, err := db.Exec(
		ctx,
		`INSERT INTO users (
			id, email, password_hash,
			first_name, last_name, phone,
			is_active, is_verified
		) VALUES (
			$1, $2, 'test-only-password-hash',
			'Settlement', 'Security', '0000000000',
			TRUE, TRUE
		)`,
		userID,
		"settlement-security-"+userID+"@connect.test",
	)
	if err != nil {
		t.Fatalf("create disposable security user: %v", err)
	}

	t.Cleanup(func() {
		_, cleanupErr := db.Exec(
			context.Background(),
			"DELETE FROM user_roles WHERE user_id = $1",
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup security user roles: %v", cleanupErr)
		}

		_, cleanupErr = db.Exec(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup security user: %v", cleanupErr)
		}
	})

	if roleName != "" {
		result, err := db.Exec(
			ctx,
			`INSERT INTO user_roles (user_id, role_id)
			 SELECT $1, id
			 FROM roles
			 WHERE name = $2`,
			userID,
			roleName,
		)
		if err != nil {
			t.Fatalf("assign test role %s: %v", roleName, err)
		}

		if result.RowsAffected() != 1 {
			t.Fatalf("test role %s not found", roleName)
		}
	}

	return userID
}

func settlementHealthSecurityToken(
	t *testing.T,
	db *pgxpool.Pool,
	jwtService *security.JWTService,
	userID string,
) string {
	t.Helper()

	user, err := repository.NewUserRepository(db).GetByID(
		context.Background(),
		userID,
	)
	if err != nil {
		t.Fatalf("load test user: %v", err)
	}

	token, err := jwtService.GenerateAccessToken(user)
	if err != nil {
		t.Fatalf("generate test JWT: %v", err)
	}

	return token
}

func settlementHealthSecurityRequest(
	router *gin.Engine,
	path string,
	token string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestFlexSettlementHealthRealJWTAndRBAC(t *testing.T) {
	db := settlementHealthSecurityDatabase(t)
	router, jwtService := settlementHealthSecurityRouter(db)

	// A missing token must be rejected before reaching the handler.
	t.Run("missing JWT", func(t *testing.T) {
		recorder := settlementHealthSecurityRequest(
			router, settlementHealthTestPath, "",
		)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s",
				recorder.Code, recorder.Body.String())
		}
	})

	t.Run("invalid JWT", func(t *testing.T) {
		recorder := settlementHealthSecurityRequest(
			router, settlementHealthTestPath, "invalid.jwt.token",
		)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s",
				recorder.Code, recorder.Body.String())
		}
	})

	// An authenticated user without any role cannot read health data.
	t.Run("no permission", func(t *testing.T) {
		userID := settlementHealthSecurityUser(t, db, "")
		token := settlementHealthSecurityToken(
			t, db, jwtService, userID,
		)

		recorder := settlementHealthSecurityRequest(
			router, settlementHealthTestPath, token,
		)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s",
				recorder.Code, recorder.Body.String())
		}
	})

	// COMPANY_ADMIN has users.read but must not inherit access
	// to platform-wide financial settlement health.
	t.Run("users.read is insufficient", func(t *testing.T) {
		userID := settlementHealthSecurityUser(t, db, "COMPANY_ADMIN")
		token := settlementHealthSecurityToken(
			t, db, jwtService, userID,
		)

		recorder := settlementHealthSecurityRequest(
			router, settlementHealthTestPath, token,
		)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s",
				recorder.Code, recorder.Body.String())
		}

		ping := settlementHealthSecurityRequest(
			router, "/api/v1/admin/ping", token,
		)

		if ping.Code != http.StatusOK {
			t.Fatalf("existing admin ping expected 200, got %d: %s",
				ping.Code, ping.Body.String())
		}
	})

	t.Run("SYSTEM_ADMIN authorized", func(t *testing.T) {
		userID := settlementHealthSecurityUser(t, db, "SYSTEM_ADMIN")
		token := settlementHealthSecurityToken(
			t, db, jwtService, userID,
		)

		recorder := settlementHealthSecurityRequest(
			router, settlementHealthTestPath, token,
		)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s",
				recorder.Code, recorder.Body.String())
		}
	})
}
