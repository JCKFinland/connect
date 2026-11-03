package api

import (
	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
)

// registerStripeProvisioningRoute is called only within the
// authenticated /api/v1/drivers route group.
//
// The caller must supply the drivers.manage permission middleware.
// Missing dependencies fail closed: no route is registered.
func registerStripeProvisioningRoute(
	drivers *gin.RouterGroup,
	permission gin.HandlerFunc,
	handler *StripeProvisioningHandler,
) bool {
	if drivers == nil || permission == nil ||
		handler == nil || handler.service == nil {
		return false
	}

	drivers.POST(
		"/:id/stripe-provisioning",
		permission,
		handler.Provision,
	)
	return true
}

// RegisterStripeProvisioningRoutes attaches the optional provisioning
// endpoint to an independently authenticated driver group.
//
// It deliberately does not modify existing NewRouter or RegisterRoutes
// signatures. A nil dependency registers no route. The caller must
// supply a fully constructed, trusted provisioning handler.
func RegisterStripeProvisioningRoutes(
	router *gin.Engine,
	auth *middleware.AuthMiddleware,
	rbac *middleware.RBACMiddleware,
	handler *StripeProvisioningHandler,
) bool {
	if router == nil || auth == nil || rbac == nil ||
		handler == nil || handler.service == nil {
		return false
	}

	drivers := router.Group("/api/v1/drivers")
	drivers.Use(auth.RequireAuth())

	return registerStripeProvisioningRoute(
		drivers,
		rbac.RequirePermission("drivers.manage"),
		handler,
	)
}
