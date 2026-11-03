package main

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

// assembleStripeProvisioningRoute combines trusted coordinator
// construction with the existing protected route registration.
//
// This is an isolated assembly boundary. Production startup does not
// call it until separate readiness and activation work is completed.
//
// The caller must validate configuration and provisioning schema
// readiness before invoking this function.
//
// No Stripe SDK client is constructed here. The supplied client must
// be provided by a separately verified application boundary.
func assembleStripeProvisioningRoute(
	enabled bool,
	db *pgxpool.Pool,
	client stripelinking.StripeAccountCreateClient,
	router *gin.Engine,
	auth *middleware.AuthMiddleware,
	rbac *middleware.RBACMiddleware,
) error {
	if !enabled || db == nil || client == nil ||
		router == nil || auth == nil || rbac == nil {
		return stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	coordinator, err := assembleStripeProvisioning(
		enabled, db, client,
	)
	if err != nil {
		return err
	}
	if coordinator == nil {
		return stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	handler := api.NewStripeProvisioningHandler(coordinator)

	if !api.RegisterStripeProvisioningRoutes(
		router, auth, rbac, handler,
	) {
		return stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	return nil
}
