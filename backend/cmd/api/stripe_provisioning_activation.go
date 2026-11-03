package main

import "errors"

// errStripeProvisioningRouteNotWired marks a feature that has passed
// configuration validation but has no production HTTP route assembly.
//
// Until trusted coordinator construction, authentication, authorization,
// and route registration are wired together, enabling the feature must
// not allow the application to start in a misleading partial state.
var errStripeProvisioningRouteNotWired = errors.New(
	"Stripe Connect provisioning is enabled but production route assembly is not wired",
)

// validateStripeProvisioningActivation is intentionally independent of
// Stripe clients and database access. It cannot make provider requests.
func validateStripeProvisioningActivation(enabled bool) error {
	if enabled {
		return errStripeProvisioningRouteNotWired
	}
	return nil
}
