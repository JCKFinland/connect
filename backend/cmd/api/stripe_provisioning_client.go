package main

import (
	"errors"

	stripe "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/config"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

var errStripeProvisioningClientUnavailable = errors.New(
	"Stripe provisioning client unavailable",
)

// newStripeProvisioningClient constructs only the Stripe account
// creation service required by the provisioning coordinator.
//
// Configuration is validated before SDK construction. The returned
// error never includes the Stripe secret key.
//
// This function does not make network requests, register HTTP routes,
// create Stripe accounts, or authorize financial operations.
//
// Production startup must independently enforce schema readiness
// and the fail-closed activation boundary.
func newStripeProvisioningClient(
	provisioning config.StripeConnectProvisioningConfig,
	stripeConfig config.StripeConfig,
) (stripelinking.StripeAccountCreateClient, error) {
	if err := config.ValidateStripeConnectProvisioningStartup(
		provisioning, stripeConfig,
	); err != nil {
		return nil, err
	}

	enabled, err := provisioning.Enabled()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, errStripeProvisioningClientUnavailable
	}

	client := stripe.NewClient(stripeConfig.SecretKey)
	if client == nil || client.V1Accounts == nil {
		return nil, errStripeProvisioningClientUnavailable
	}

	return client.V1Accounts, nil
}
