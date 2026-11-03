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
