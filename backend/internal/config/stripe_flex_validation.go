package config

import (
	"errors"
	"strings"
)

var ErrInvalidStripeFlexConfiguration = errors.New(
	"invalid Stripe FLEX webhook configuration",
)

// ValidateStripeFlexConfiguration ensures that FLEX webhook
// reconciliation cannot be enabled without its Stripe API key.
//
// The existing payment callback configuration remains independent.
func ValidateStripeFlexConfiguration(cfg StripeConfig) error {
	secretKeyConfigured := strings.TrimSpace(cfg.SecretKey) != ""
	flexWebhookConfigured := strings.TrimSpace(cfg.FlexWebhookSecret) != ""

	if flexWebhookConfigured && !secretKeyConfigured {
		return ErrInvalidStripeFlexConfiguration
	}

	return nil
}
