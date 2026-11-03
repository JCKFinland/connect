package main

import (
	"errors"
	"testing"
)

func TestStripeProvisioningActivationDisabledAllowed(t *testing.T) {
	if err := validateStripeProvisioningActivation(false); err != nil {
		t.Fatalf("disabled provisioning must remain allowed: %v", err)
	}
}

func TestStripeProvisioningActivationEnabledRejected(t *testing.T) {
	err := validateStripeProvisioningActivation(true)
	if !errors.Is(err, errStripeProvisioningRouteNotWired) {
		t.Fatalf("enabled provisioning must fail closed: %v", err)
	}
}
