package main

import (
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

func TestStripeProvisioningRouteAssemblyRejectsIncompleteDependencies(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	// These are constructor-only placeholders.
	// No database connection or provider request is made.
	db := &pgxpool.Pool{}
	client := assemblyInertStripeClient{}
	auth := &middleware.AuthMiddleware{}
	rbac := &middleware.RBACMiddleware{}

	cases := []struct {
		name    string
		enabled bool
		db      *pgxpool.Pool
		client  stripelinking.StripeAccountCreateClient
		router  *gin.Engine
		auth    *middleware.AuthMiddleware
		rbac    *middleware.RBACMiddleware
	}{
		{
			name:    "disabled",
			enabled: false,
			db:      db,
			client:  client,
			router:  gin.New(),
			auth:    auth,
			rbac:    rbac,
		},
		{
			name:    "missing_database",
			enabled: true,
			client:  client,
			router:  gin.New(),
			auth:    auth,
			rbac:    rbac,
		},
		{
			name:    "missing_client",
			enabled: true,
			db:      db,
			router:  gin.New(),
			auth:    auth,
			rbac:    rbac,
		},
		{
			name:    "missing_router",
			enabled: true,
			db:      db,
			client:  client,
			auth:    auth,
			rbac:    rbac,
		},
		{
			name:    "missing_auth",
			enabled: true,
			db:      db,
			client:  client,
			router:  gin.New(),
			rbac:    rbac,
		},
		{
			name:    "missing_rbac",
			enabled: true,
			db:      db,
			client:  client,
			router:  gin.New(),
			auth:    auth,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := assembleStripeProvisioningRoute(
				tc.enabled, tc.db, tc.client,
				tc.router, tc.auth, tc.rbac,
			)

			if !errors.Is(
				err,
				stripelinking.ErrStripeProvisioningCompositionUnavailable,
			) {
				t.Fatalf("expected fail-closed composition error, got %v", err)
			}

			if tc.router != nil && len(tc.router.Routes()) != 0 {
				t.Fatalf("unexpected routes: %+v", tc.router.Routes())
			}
		})
	}
}

func TestStripeProvisioningRouteAssemblyRegistersProtectedEndpoint(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	// Constructor-only placeholders: do not issue HTTP requests
	// with these uninitialized middleware dependencies.
	err := assembleStripeProvisioningRoute(
		true,
		&pgxpool.Pool{},
		assemblyInertStripeClient{},
		router,
		&middleware.AuthMiddleware{},
		&middleware.RBACMiddleware{},
	)
	if err != nil {
		t.Fatalf("trusted route assembly failed: %v", err)
	}

	routes := router.Routes()
	if len(routes) != 1 {
		t.Fatalf("expected one protected route, got %+v", routes)
	}

	if routes[0].Method != "POST" ||
		routes[0].Path != "/api/v1/drivers/:id/stripe-provisioning" {
		t.Fatalf("unexpected provisioning route: %+v", routes[0])
	}
}
