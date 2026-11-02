package stripelinking_test

import (
	"testing"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/rbac"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

// Compile-time checks ensure production authorization dependencies
// satisfy the coordinator contracts without introducing adapters.
var (
	_ stripelinking.PermissionChecker = (*rbac.Service)(nil)
	_ stripelinking.PermissionChecker = (*repository.PostgresPermissionRepository)(nil)
	_ stripelinking.MembershipChecker = (*postgresrepo.CompanyMembershipRepository)(nil)
	_ stripelinking.DriverResolver    = (*postgresrepo.DriverRepository)(nil)
)

// This test intentionally does not construct Stripe credentials,
// create a provider account, register routes or mutate the database.
func TestStripeProvisioningAuthorizationWiringContract(t *testing.T) {
	t.Log("CONNECT authorization interfaces compile against existing implementations")
}
