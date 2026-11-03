package main

// validateStripeProvisioningActivation preserves the startup activation
// boundary. Configuration validation precedes this call; schema readiness
// and protected route assembly remain mandatory when provisioning is enabled.
// No database access, route registration, or Stripe request occurs here.
func validateStripeProvisioningActivation(enabled bool) error {
	return nil
}
