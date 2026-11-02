package api

import "github.com/gin-gonic/gin"

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
