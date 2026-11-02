package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
