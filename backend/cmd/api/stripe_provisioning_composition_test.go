package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

const compositionSyntheticKey = "sk_test_connect_composition_synthetic_only"

func compositionRouteCount(router *gin.Engine) int {
	count := 0
	for _, route := range router.Routes() {
		if route.Method == "POST" &&
			route.Path == "/api/v1/drivers/:id/stripe-provisioning" {
			count++
		}
	}
	return count
}

func TestStripeProvisioningCompositionDisabled(t *testing.T) {
	router := gin.New()

	err := assembleStripeProvisioningComposition(
		config.StripeConnectProvisioningConfig{
			EnabledRaw: "false",
		},
		config.StripeConfig{},
		&pgxpool.Pool{},
		router,
		&middleware.AuthMiddleware{},
		&middleware.RBACMiddleware{},
	)

	if !errors.Is(
		err, stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("expected fail-closed composition, got %v", err)
	}
	if compositionRouteCount(router) != 0 {
		t.Fatal("disabled provisioning registered a route")
	}
}

func TestStripeProvisioningCompositionRejectsInvalidConfiguration(
	t *testing.T,
) {
	cases := []struct {
		name    string
		enabled string
		key     string
	}{
		{"invalid_flag", "TRUE", compositionSyntheticKey},
		{"missing_key", "true", ""},
		{"invalid_key", "true", "pk_test_synthetic_only"},
		{"whitespace_key", "true", " sk_test_synthetic_only"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()

			err := assembleStripeProvisioningComposition(
				config.StripeConnectProvisioningConfig{
					EnabledRaw: tc.enabled,
				},
				config.StripeConfig{
					SecretKey: tc.key,
				},
				&pgxpool.Pool{},
				router,
				&middleware.AuthMiddleware{},
				&middleware.RBACMiddleware{},
			)

			if err == nil {
				t.Fatal("invalid configuration was accepted")
			}
			if compositionRouteCount(router) != 0 {
				t.Fatal("invalid configuration registered a route")
			}
			if tc.key != "" && strings.Contains(err.Error(), tc.key) {
				t.Fatal("composition error exposed credential")
			}
		})
	}
}

func TestStripeProvisioningCompositionRejectsMissingDependencies(
	t *testing.T,
) {
	cases := []struct {
		name        string
		missingDB   bool
		missingAPI  bool
		missingAuth bool
		missingRBAC bool
	}{
		{name: "missing_database", missingDB: true},
		{name: "missing_router", missingAPI: true},
		{name: "missing_auth", missingAuth: true},
		{name: "missing_rbac", missingRBAC: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &pgxpool.Pool{}
			router := gin.New()
			auth := &middleware.AuthMiddleware{}
			rbac := &middleware.RBACMiddleware{}

			if tc.missingDB {
				db = nil
			}
			if tc.missingAPI {
				router = nil
			}
			if tc.missingAuth {
				auth = nil
			}
			if tc.missingRBAC {
				rbac = nil
			}

			err := assembleStripeProvisioningComposition(
				config.StripeConnectProvisioningConfig{
					EnabledRaw: "true",
				},
				config.StripeConfig{
					SecretKey: compositionSyntheticKey,
				},
				db, router, auth, rbac,
			)

			if !errors.Is(
				err,
				stripelinking.ErrStripeProvisioningCompositionUnavailable,
			) {
				t.Fatalf("expected unavailable error, got %v", err)
			}
			if router != nil && compositionRouteCount(router) != 0 {
				t.Fatal("missing dependency registered a route")
			}
		})
	}
}

func TestStripeProvisioningCompositionRegistersProtectedRoute(
	t *testing.T,
) {
	// Synthetic key, inert DB pool and middleware placeholders.
	// The test inspects registration only: no HTTP request is sent.
	router := gin.New()

	err := assembleStripeProvisioningComposition(
		config.StripeConnectProvisioningConfig{
			EnabledRaw: "true",
		},
		config.StripeConfig{
			SecretKey: compositionSyntheticKey,
		},
		&pgxpool.Pool{},
		router,
		&middleware.AuthMiddleware{},
		&middleware.RBACMiddleware{},
	)

	if err != nil {
		t.Fatalf("composition failed: %v", err)
	}
	if count := compositionRouteCount(router); count != 1 {
		t.Fatalf("expected one protected route, got %d", count)
	}
}
