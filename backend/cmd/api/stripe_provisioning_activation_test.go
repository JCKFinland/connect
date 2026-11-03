package main

import "testing"

func TestStripeProvisioningActivationDisabledAllowed(t *testing.T) {
	if err := validateStripeProvisioningActivation(false); err != nil {
		t.Fatalf("disabled provisioning must remain allowed: %v", err)
	}
}

func TestStripeProvisioningActivationEnabledAllowed(t *testing.T) {
	if err := validateStripeProvisioningActivation(true); err != nil {
		t.Fatalf("validated enabled provisioning must be allowed: %v", err)
	}
}
