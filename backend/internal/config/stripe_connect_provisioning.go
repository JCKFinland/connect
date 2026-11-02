package config

import (
	"errors"
	"fmt"
)

var ErrStripeConnectProvisioningConfiguration = errors.New(
	"invalid Stripe Connect provisioning configuration",
)

// StripeConnectProvisioningConfig is independent of Stripe payment
// execution and webhook configuration. Its default is disabled.
type StripeConnectProvisioningConfig struct {
	EnabledRaw string
}

// Enabled accepts only exact lowercase true or false.
// Invalid configuration fails closed.
func (c StripeConnectProvisioningConfig) Enabled() (bool, error) {
	switch c.EnabledRaw {
	case "true":
		return true, nil
	case "false", "":
		return false, nil
	default:
		return false, fmt.Errorf(
			"%w: STRIPE_CONNECT_PROVISIONING_ENABLED must be true or false",
			ErrStripeConnectProvisioningConfiguration,
		)
	}
}
