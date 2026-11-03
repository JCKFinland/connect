package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	stripe "github.com/stripe/stripe-go/v86"

	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

type assemblyInertStripeClient struct{}

func (assemblyInertStripeClient) Create(
	context.Context,
	*stripe.AccountCreateParams,
) (*stripe.Account, error) {
	return nil, errors.New("Stripe network disabled in assembly test")
}

var _ stripelinking.StripeAccountCreateClient = assemblyInertStripeClient{}

func TestStripeProvisioningApplicationAssemblyDisabled(t *testing.T) {
	coordinator, err := assembleStripeProvisioning(
		false,
		nil,
		nil,
	)
	if coordinator != nil ||
		!errors.Is(err, stripelinking.ErrStripeProvisioningCompositionUnavailable) {
		t.Fatalf("disabled assembly did not fail closed: %v", err)
	}
}

func TestStripeProvisioningApplicationAssemblyRejectsNilDatabase(t *testing.T) {
	coordinator, err := assembleStripeProvisioning(
		true,
		nil,
		assemblyInertStripeClient{},
	)
	if coordinator != nil ||
		!errors.Is(err, stripelinking.ErrStripeProvisioningCompositionUnavailable) {
		t.Fatalf("nil database did not fail closed: %v", err)
	}
}

func TestStripeProvisioningApplicationAssemblyRejectsMissingClient(t *testing.T) {
	// The inert pool is never connected, queried, or closed.
	coordinator, err := assembleStripeProvisioning(
		true,
		&pgxpool.Pool{},
		nil,
	)
	if coordinator != nil ||
		!errors.Is(err, stripelinking.ErrStripeProvisioningCompositionUnavailable) {
		t.Fatalf("missing Stripe client did not fail closed: %v", err)
	}
}

func TestStripeProvisioningApplicationAssemblyTrustedDependencies(t *testing.T) {
	// Constructors only: no database query or Stripe request.
	coordinator, err := assembleStripeProvisioning(
		true,
		&pgxpool.Pool{},
		assemblyInertStripeClient{},
	)
	if err != nil || coordinator == nil {
		t.Fatalf("trusted dependency assembly failed: %v", err)
	}
}
