package stripelinking_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	stripe "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/rbac"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

func TestStripeProvisioningCompositionDisabledByDefault(t *testing.T) {
	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		false,
		stripelinking.ProvisioningCoordinatorDeps{},
		nil,
	)

	if coordinator != nil {
		t.Fatal("disabled composition returned a coordinator")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected disabled error: %v", err)
	}
}

func TestStripeProvisioningCompositionRejectsMissingDependencies(t *testing.T) {
	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		true,
		stripelinking.ProvisioningCoordinatorDeps{},
		nil,
	)

	if coordinator != nil {
		t.Fatal("incomplete composition returned a coordinator")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected incomplete error: %v", err)
	}
}

type compositionAccountClient struct{}

func (compositionAccountClient) Create(
	context.Context,
	*stripe.AccountCreateParams,
) (*stripe.Account, error) {
	return nil, errors.New("network disabled in composition test")
}

var _ stripelinking.StripeAccountCreateClient = compositionAccountClient{}

func trustedCompositionDeps() stripelinking.ProvisioningCoordinatorDeps {
	var db *pgxpool.Pool

	permissions := rbac.NewService(
		repository.NewPermissionRepository(db),
	)

	return stripelinking.ProvisioningCoordinatorDeps{
		Permissions: permissions,
		Memberships: postgresrepo.NewCompanyMembershipRepository(db),
		Drivers:     postgresrepo.NewDriverRepository(db),
		DB:          db,
	}
}

func TestStripeProvisioningCompositionRejectsNilDatabase(t *testing.T) {
	deps := trustedCompositionDeps()

	// A nil DB must always fail closed, even when the other
	// production authorization interfaces are present.
	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		true,
		deps,
		compositionAccountClient{},
	)

	if coordinator != nil {
		t.Fatal("nil database unexpectedly enabled provisioning")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected nil database error: %v", err)
	}
}

func TestStripeProvisioningCompositionRejectsInjectedAccountCreator(t *testing.T) {
	deps := trustedCompositionDeps()
	deps.DB = &pgxpool.Pool{}
	deps.Accounts = compositionInertCreator{}

	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		true,
		deps,
		compositionAccountClient{},
	)

	if coordinator != nil {
		t.Fatal("conflicting account creator unexpectedly accepted")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected conflicting creator error: %v", err)
	}
}

type compositionInertCreator struct{}

func (compositionInertCreator) CreateProvisioningAccount(
	context.Context, string, string, string, string,
) (string, error) {
	return "", errors.New("provider disabled")
}

// The inert pool is never connected or queried.
// The fake Stripe client is never called.
func TestStripeProvisioningCompositionEnabledWithTrustedDependencies(t *testing.T) {
	deps := trustedCompositionDeps()
	deps.DB = &pgxpool.Pool{}

	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		true,
		deps,
		compositionAccountClient{},
	)

	if err != nil {
		t.Fatalf("complete composition rejected: %v", err)
	}

	if coordinator == nil {
		t.Fatal("complete composition returned nil coordinator")
	}
}

func TestStripeProvisioningCompositionRejectsMissingStripeClient(t *testing.T) {
	deps := trustedCompositionDeps()
	deps.DB = &pgxpool.Pool{}

	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		true,
		deps,
		nil,
	)

	if coordinator != nil {
		t.Fatal("missing Stripe client unexpectedly accepted")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected missing client error: %v", err)
	}
}
