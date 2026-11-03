package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
)

type routeOnlyProvisioningService struct{}

func (routeOnlyProvisioningService) ProvisionAccount(
	context.Context, string, string,
) (string, error) {
	return "", nil
}

func TestStripeProvisioningRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	permission := func(c *gin.Context) {
		c.Next()
	}

	t.Run("disabled_without_handler", func(t *testing.T) {
		router := gin.New()
		drivers := router.Group("/api/v1/drivers")

		if registerStripeProvisioningRoute(
			drivers, permission, nil,
		) {
			t.Fatal("nil handler registered route")
		}
		if len(router.Routes()) != 0 {
			t.Fatalf("unexpected routes: %+v", router.Routes())
		}
	})

	t.Run("disabled_without_permission", func(t *testing.T) {
		router := gin.New()
		drivers := router.Group("/api/v1/drivers")
		handler := NewStripeProvisioningHandler(
			routeOnlyProvisioningService{},
		)

		if registerStripeProvisioningRoute(
			drivers, nil, handler,
		) {
			t.Fatal("missing permission middleware registered route")
		}
		if len(router.Routes()) != 0 {
			t.Fatalf("unexpected routes: %+v", router.Routes())
		}
	})

	t.Run("disabled_without_service", func(t *testing.T) {
		router := gin.New()
		drivers := router.Group("/api/v1/drivers")
		handler := NewStripeProvisioningHandler(nil)

		if registerStripeProvisioningRoute(
			drivers, permission, handler,
		) {
			t.Fatal("missing service registered route")
		}
		if len(router.Routes()) != 0 {
			t.Fatalf("unexpected routes: %+v", router.Routes())
		}
	})

	t.Run("enabled_with_complete_dependencies", func(t *testing.T) {
		router := gin.New()
		drivers := router.Group("/api/v1/drivers")
		handler := NewStripeProvisioningHandler(
			routeOnlyProvisioningService{},
		)

		if !registerStripeProvisioningRoute(
			drivers, permission, handler,
		) {
			t.Fatal("valid handler did not register route")
		}

		routes := router.Routes()
		if len(routes) != 1 {
			t.Fatalf("expected one route, got %+v", routes)
		}
		if routes[0].Method != "POST" ||
			routes[0].Path != "/api/v1/drivers/:id/stripe-provisioning" {
			t.Fatalf("unexpected route: %+v", routes[0])
		}
	})
}

// TestStripeProvisioningRoutePermissionEnforcement verifies that
// the route's permission middleware blocks access before the
// provisioning handler is invoked.
func TestStripeProvisioningRoutePermissionEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const path = "/api/v1/drivers/123/stripe-provisioning"

	t.Run("permission_denied", func(t *testing.T) {
		router := gin.New()
		drivers := router.Group("/api/v1/drivers")

		handler := NewStripeProvisioningHandler(
			routeOnlyProvisioningService{},
		)

		permissionCalls := 0
		permission := func(c *gin.Context) {
			permissionCalls++
			c.AbortWithStatus(http.StatusForbidden)
		}

		if !registerStripeProvisioningRoute(
			drivers, permission, handler,
		) {
			t.Fatal("expected protected route registration")
		}

		request := httptest.NewRequest(
			http.MethodPost, path, nil,
		)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if permissionCalls != 1 {
			t.Fatalf(
				"permission middleware called %d times",
				permissionCalls,
			)
		}

		if recorder.Code != http.StatusForbidden {
			t.Fatalf(
				"status=%d, want 403",
				recorder.Code,
			)
		}

		if strings.Contains(
			recorder.Body.String(),
			"stripe_account_id",
		) {
			t.Fatal("denied request exposed account response")
		}
	})

	t.Run("permission_allowed_reaches_handler", func(t *testing.T) {
		router := gin.New()
		drivers := router.Group("/api/v1/drivers")

		permissionCalls := 0
		permission := func(c *gin.Context) {
			permissionCalls++
			c.Next()
		}

		handler := NewStripeProvisioningHandler(
			routeOnlyProvisioningService{},
		)

		if !registerStripeProvisioningRoute(
			drivers, permission, handler,
		) {
			t.Fatal("expected protected route registration")
		}

		request := httptest.NewRequest(
			http.MethodPost, path, nil,
		)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if permissionCalls != 1 {
			t.Fatalf(
				"permission middleware called %d times",
				permissionCalls,
			)
		}

		// Authentication is deliberately absent in this isolated
		// test, so the reached handler must return 401.
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"status=%d, want 401 from provisioning handler",
				recorder.Code,
			)
		}
	})
}

// TestStripeProvisioningProductionIntegrationFailClosed proves that
// incomplete production dependencies cannot expose provisioning.
func TestStripeProvisioningProductionIntegrationFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewStripeProvisioningHandler(
		routeOnlyProvisioningService{},
	)

	cases := []struct {
		name    string
		handler *StripeProvisioningHandler
	}{
		{"missing_all_dependencies", nil},
		{"handler_without_auth_or_rbac", handler},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()

			if RegisterStripeProvisioningRoutes(
				router, nil, nil, tc.handler,
			) {
				t.Fatal("incomplete dependencies registered route")
			}

			for _, route := range router.Routes() {
				if route.Path == "/api/v1/drivers/:id/stripe-provisioning" {
					t.Fatal("provisioning endpoint exposed")
				}
			}
		})
	}
}

// TestStripeProvisioningProductionIntegrationDependencies verifies
// that each missing dependency independently prevents registration.
// No real credentials, database writes, or provider calls are used.
func TestStripeProvisioningProductionIntegrationDependencies(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	handler := NewStripeProvisioningHandler(
		routeOnlyProvisioningService{},
	)
	emptyHandler := NewStripeProvisioningHandler(nil)

	// These middleware objects are intentionally not invoked.
	// Each case must return before middleware construction.
	auth := &middleware.AuthMiddleware{}
	rbac := &middleware.RBACMiddleware{}

	cases := []struct {
		name    string
		router  *gin.Engine
		auth    *middleware.AuthMiddleware
		rbac    *middleware.RBACMiddleware
		handler *StripeProvisioningHandler
	}{
		{
			name: "missing_router",
			auth: auth, rbac: rbac, handler: handler,
		},
		{
			name:   "missing_auth",
			router: gin.New(), rbac: rbac, handler: handler,
		},
		{
			name:   "missing_rbac",
			router: gin.New(), auth: auth, handler: handler,
		},
		{
			name:   "missing_handler",
			router: gin.New(), auth: auth, rbac: rbac,
		},
		{
			name:   "missing_service",
			router: gin.New(), auth: auth, rbac: rbac,
			handler: emptyHandler,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if RegisterStripeProvisioningRoutes(
				tc.router, tc.auth, tc.rbac, tc.handler,
			) {
				t.Fatal("incomplete dependencies registered route")
			}

			if tc.router != nil && len(tc.router.Routes()) != 0 {
				t.Fatalf(
					"unexpected routes: %+v",
					tc.router.Routes(),
				)
			}
		})
	}
}
