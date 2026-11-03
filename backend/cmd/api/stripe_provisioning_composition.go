package main

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

// assembleStripeProvisioningComposition connects the separately verified
// configuration, SDK client, coordinator, and protected route boundaries.
//
// This is an isolated composition entry point. Production startup does
// not invoke it. Schema readiness and production activation are separate
// prerequisites and are deliberately not bypassed here.
//
// The function performs no Stripe API requests, database migrations,
// payout operations, or HTTP server startup.
func assembleStripeProvisioningComposition(
	provisioning config.StripeConnectProvisioningConfig,
	stripeConfig config.StripeConfig,
	db *pgxpool.Pool,
	router *gin.Engine,
	auth *middleware.AuthMiddleware,
	rbac *middleware.RBACMiddleware,
) error {
	enabled, err := provisioning.Enabled()
	if err != nil {
		return err
	}

	if !enabled {
		return stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	// Check structural dependencies before constructing an SDK client
	// or registering any route.
	if db == nil || router == nil || auth == nil || rbac == nil {
		return stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	client, err := newStripeProvisioningClient(
		provisioning, stripeConfig,
	)
	if err != nil {
		return err
	}
	if client == nil {
		return stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	return assembleStripeProvisioningRoute(
		enabled, db, client, router, auth, rbac,
	)
}
