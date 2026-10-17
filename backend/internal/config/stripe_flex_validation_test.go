package config

import (
	"errors"
	"testing"
)

func TestValidateStripeFlexConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		config      StripeConfig
		expectError bool
	}{
		{
			name:   "all Stripe configuration absent",
			config: StripeConfig{},
		},
		{
			name: "existing payment execution only",
			config: StripeConfig{
				SecretKey: "sk_test_example",
			},
		},
		{
			name: "existing payment callbacks only",
			config: StripeConfig{
				WebhookSecret: "whsec_existing",
			},
		},
		{
			name: "existing Stripe payment configuration",
			config: StripeConfig{
				SecretKey:     "sk_test_example",
				WebhookSecret: "whsec_existing",
			},
		},
		{
			name: "FLEX webhook correctly configured",
			config: StripeConfig{
				SecretKey:         "sk_test_example",
				FlexWebhookSecret: "whsec_flex",
			},
		},
		{
			name: "existing and FLEX webhooks configured",
			config: StripeConfig{
				SecretKey:         "sk_test_example",
				WebhookSecret:     "whsec_existing",
				FlexWebhookSecret: "whsec_flex",
			},
		},
		{
			name: "FLEX webhook missing Stripe API key",
			config: StripeConfig{
				FlexWebhookSecret: "whsec_flex",
			},
			expectError: true,
		},
		{
			name: "FLEX webhook with whitespace API key",
			config: StripeConfig{
				SecretKey:         "   ",
				FlexWebhookSecret: "whsec_flex",
			},
			expectError: true,
		},
		{
			name: "whitespace FLEX secret is disabled",
			config: StripeConfig{
				FlexWebhookSecret: "   ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStripeFlexConfiguration(tt.config)

			if tt.expectError {
				if !errors.Is(
					err,
					ErrInvalidStripeFlexConfiguration,
				) {
					t.Fatalf(
						"expected invalid FLEX configuration, got %v",
						err,
					)
				}
				return
			}

			if err != nil {
				t.Fatalf(
					"expected valid configuration, got %v",
					err,
				)
			}
		})
	}
}
