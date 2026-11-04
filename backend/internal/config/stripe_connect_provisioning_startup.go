package config

import (
	"fmt"
	"strings"
)

// ValidateStripeConnectProvisioningStartup validates the provisioning
// configuration before database initialization or HTTP startup.
//
// This does not enable provisioning, construct a Stripe client,
// register a route, or authorize any financial operation.
func ValidateStripeConnectProvisioningStartup(
	provisioning StripeConnectProvisioningConfig,
	stripe StripeConfig,
) error {
	enabled, err := provisioning.Enabled()
	if err != nil {
		return err
	}

	if !enabled {
		return nil
	}

	key := stripe.SecretKey
	if key == "" ||
		key != strings.TrimSpace(key) ||
		strings.ContainsAny(key, " \t\r\n") ||
		!(strings.HasPrefix(key, "sk_test_") ||
			strings.HasPrefix(key, "sk_live_")) ||
		len(key) <= len("sk_test_") {
		return fmt.Errorf(
			"%w: valid STRIPE_SECRET_KEY required when provisioning enabled",
			ErrStripeConnectProvisioningConfiguration,
		)
	}

	return nil
}

// ValidateStripeConnectProvisioningEnvironment enforces the Stripe
// credential mode appropriate for the application environment.
// Disabled provisioning does not require credentials or environment checks.
func ValidateStripeConnectProvisioningEnvironment(
	provisioning StripeConnectProvisioningConfig,
	stripe StripeConfig,
	environment string,
) error {
	if err := ValidateStripeConnectProvisioningStartup(
		provisioning, stripe,
	); err != nil {
		return err
	}

	enabled, err := provisioning.Enabled()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	switch environment {
	case "development", "test":
		if !strings.HasPrefix(stripe.SecretKey, "sk_test_") {
			return fmt.Errorf(
				"%w: non-production provisioning requires Stripe test-mode secret key",
				ErrStripeConnectProvisioningConfiguration,
			)
		}
	case "production":
		if !strings.HasPrefix(stripe.SecretKey, "sk_live_") {
			return fmt.Errorf(
				"%w: production provisioning requires Stripe live-mode secret key",
				ErrStripeConnectProvisioningConfiguration,
			)
		}
	default:
		return fmt.Errorf(
			"%w: unsupported application environment",
			ErrStripeConnectProvisioningConfiguration,
		)
	}

	return nil
}
