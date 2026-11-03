package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
)

func TestStripeProvisioningClientFactoryDisabled(t *testing.T) {
	client, err := newStripeProvisioningClient(
		config.StripeConnectProvisioningConfig{
			EnabledRaw: "false",
		},
		config.StripeConfig{},
	)

	if client != nil {
		t.Fatal("disabled provisioning returned a Stripe client")
	}
	if !errors.Is(err, errStripeProvisioningClientUnavailable) {
		t.Fatalf("expected disabled factory error, got %v", err)
	}
}

func TestStripeProvisioningClientFactoryRejectsInvalidConfiguration(
	t *testing.T,
) {
	cases := []struct {
		name    string
		enabled string
		key     string
	}{
		{"invalid_flag", "TRUE", "sk_test_synthetic_only"},
		{"empty_key", "true", ""},
		{"wrong_prefix", "true", "pk_test_synthetic_only"},
		{"whitespace", "true", "sk_test_synthetic only"},
		{"surrounding_whitespace", "true", " sk_test_synthetic_only"},
		{"short_key", "true", "sk_test_"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := newStripeProvisioningClient(
				config.StripeConnectProvisioningConfig{
					EnabledRaw: tc.enabled,
				},
				config.StripeConfig{
					SecretKey: tc.key,
				},
			)

			if client != nil || err == nil {
				t.Fatal("invalid configuration did not fail closed")
			}
			if tc.key != "" && strings.Contains(err.Error(), tc.key) {
				t.Fatal("configuration error exposed credential")
			}
		})
	}
}

func TestStripeProvisioningClientFactoryConstructsWithoutNetwork(
	t *testing.T,
) {
	// Synthetic credential only. No Stripe SDK method is invoked.
	// Client construction itself must not issue HTTP requests.
	client, err := newStripeProvisioningClient(
		config.StripeConnectProvisioningConfig{
			EnabledRaw: "true",
		},
		config.StripeConfig{
			SecretKey: "sk_test_connect_synthetic_factory_only",
		},
	)

	if err != nil {
		t.Fatalf("factory construction failed: %v", err)
	}
	if client == nil {
		t.Fatal("factory returned nil account creation service")
	}
}
