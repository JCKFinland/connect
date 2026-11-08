package main

import (
	"errors"
	"testing"
)

func TestStripeProvisioningActivationDisabledAllowed(t *testing.T) {
	t.Setenv(
		"CONNECT_STRIPE_PROVISIONING_PRODUCTION_AUTHORIZED",
		"",
	)

	if err := validateStripeProvisioningActivation(
		false, "production",
	); err != nil {
		t.Fatalf("disabled provisioning must remain allowed: %v", err)
	}
}

func TestStripeProvisioningActivationEnabledAllowed(t *testing.T) {
	t.Setenv(
		"CONNECT_STRIPE_PROVISIONING_PRODUCTION_AUTHORIZED",
		"",
	)

	for _, environment := range []string{
		"development", "test",
	} {
		t.Run(environment, func(t *testing.T) {
			if err := validateStripeProvisioningActivation(
				true, environment,
			); err != nil {
				t.Fatalf(
					"non-production provisioning must remain allowed: %v",
					err,
				)
			}
		})
	}
}

func TestStripeProvisioningProductionActivationFailsClosed(t *testing.T) {
	for _, value := range []string{
		"", "false", "TRUE", "1", "yes", " true ",
	} {
		t.Run("value_"+value, func(t *testing.T) {
			t.Setenv(
				"CONNECT_STRIPE_PROVISIONING_PRODUCTION_AUTHORIZED",
				value,
			)

			err := validateStripeProvisioningActivation(
				true, "production",
			)

			if !errors.Is(
				err,
				errStripeProductionActivationUnauthorized,
			) {
				t.Fatalf(
					"expected unauthorized production activation, got %v",
					err,
				)
			}
		})
	}
}

func TestStripeProvisioningProductionActivationAuthorized(t *testing.T) {
	t.Setenv(
		"CONNECT_STRIPE_PROVISIONING_PRODUCTION_AUTHORIZED",
		"true",
	)

	if err := validateStripeProvisioningActivation(
		true, "production",
	); err != nil {
		t.Fatalf(
			"explicitly authorized activation must pass guard: %v",
			err,
		)
	}
}
