package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/rbac"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

// assembleStripeProvisioning constructs trusted application dependencies.
//
// It does not create a Stripe SDK client, contact Stripe, register an HTTP
// route, migrate the database, or authorize a payout.
//
// The caller must separately validate configuration and schema readiness
// before enabling this composition.
func assembleStripeProvisioning(
	enabled bool,
	db *pgxpool.Pool,
	client stripelinking.StripeAccountCreateClient,
) (*stripelinking.ProvisioningCoordinator, error) {
	if !enabled || db == nil || client == nil {
		return nil, stripelinking.ErrStripeProvisioningCompositionUnavailable
	}

	permissionRepo := repository.NewPermissionRepository(db)
	permissionService := rbac.NewService(permissionRepo)

	deps := stripelinking.ProvisioningCoordinatorDeps{
		Permissions: permissionService,
		Memberships: postgresrepo.NewCompanyMembershipRepository(db),
		Drivers:     postgresrepo.NewDriverRepository(db),
		DB:          db,
	}

	return stripelinking.NewProvisioningCoordinatorIfEnabled(
		enabled,
		deps,
		client,
	)
}
