package stripelinking_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/rbac"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

// An inert provider prevents external account creation.
type inertProvisioningCreator struct{}

func (inertProvisioningCreator) CreateProvisioningAccount(
	context.Context, string, string, string, string,
) (string, error) {
	return "", errors.New("provider disabled in assembly test")
}

var _ stripelinking.ProvisioningAccountCreator = inertProvisioningCreator{}

// This assembly is deliberately not invoked with a live DB pool.
// Constructors only: no query, migration, Stripe call or route.
func assembleStripeProvisioningForTest(
	db *pgxpool.Pool,
) *stripelinking.ProvisioningCoordinator {
	permissionRepo := repository.NewPermissionRepository(db)
	permissionService := rbac.NewService(permissionRepo)
	membershipRepo := postgresrepo.NewCompanyMembershipRepository(db)
	driverRepo := postgresrepo.NewDriverRepository(db)

	return stripelinking.NewProvisioningCoordinator(
		stripelinking.ProvisioningCoordinatorDeps{
			Permissions: permissionService,
			Memberships: membershipRepo,
			Drivers:     driverRepo,
			DB:          db,
			Accounts:    inertProvisioningCreator{},
		},
	)
}

func TestStripeProvisioningDependencyAssembly(t *testing.T) {
	// A nil pool is safe here because no coordinator operation runs.
	coordinator := assembleStripeProvisioningForTest(nil)
	if coordinator == nil {
		t.Fatal("coordinator constructor returned nil")
	}
}
