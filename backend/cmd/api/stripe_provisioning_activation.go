package main

import (
	"errors"
	"os"
)

var errStripeProductionActivationUnauthorized = errors.New(
	"production Stripe provisioning activation is not authorized",
)

// validateStripeProvisioningActivation is an early startup safety boundary.
// It runs before database initialization and automatic migrations.
//
// Disabled provisioning and non-production provisioning preserve their
// existing behavior. Production provisioning requires an explicit,
// independent activation authorization.
//
// This guard does not connect to a database, create a Stripe client,
// register routes, or perform any Stripe request.
func validateStripeProvisioningActivation(
	enabled bool,
	environment string,
) error {
	if !enabled || environment != "production" {
		return nil
	}

	if os.Getenv("CONNECT_STRIPE_PROVISIONING_PRODUCTION_AUTHORIZED") != "true" {
		return errStripeProductionActivationUnauthorized
	}

	return nil
}
