package config

import (
	"errors"
	"testing"
)

func TestStripeConnectProvisioningStartupCredentialGate(t *testing.T) {
	cases := []struct {
		name    string
		flag    string
		key     string
		wantErr bool
	}{
		{
			name: "default_disabled_without_key",
		},
		{
			name: "explicit_disabled_without_key",
			flag: "false",
		},
		{
			name: "disabled_with_arbitrary_key",
			flag: "false",
			key:  "not-a-stripe-key",
		},
		{
			name:    "invalid_flag",
			flag:    "TRUE",
			key:     "sk_test_example",
			wantErr: true,
		},
		{
			name:    "enabled_missing_key",
			flag:    "true",
			wantErr: true,
		},
		{
			name:    "enabled_whitespace_key",
			flag:    "true",
			key:     "   ",
			wantErr: true,
		},
		{
			name:    "enabled_publishable_key",
			flag:    "true",
			key:     "pk_test_example",
			wantErr: true,
		},
		{
			name:    "enabled_restricted_key",
			flag:    "true",
			key:     "rk_test_example",
			wantErr: true,
		},
		{
			name:    "enabled_surrounding_whitespace",
			flag:    "true",
			key:     " sk_test_example ",
			wantErr: true,
		},
		{
			name:    "enabled_embedded_whitespace",
			flag:    "true",
			key:     "sk_test_example value",
			wantErr: true,
		},
		{
			name:    "enabled_empty_test_prefix",
			flag:    "true",
			key:     "sk_test_",
			wantErr: true,
		},
		{
			name: "enabled_test_key",
			flag: "true",
			key:  "sk_test_configuration_only",
		},
		{
			name: "enabled_live_key_format",
			flag: "true",
			key:  "sk_live_configuration_only",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStripeConnectProvisioningStartup(
				StripeConnectProvisioningConfig{
					EnabledRaw: tc.flag,
				},
				StripeConfig{
					SecretKey: tc.key,
				},
			)

			if tc.wantErr {
				if !errors.Is(
					err,
					ErrStripeConnectProvisioningConfiguration,
				) {
					t.Fatalf(
						"expected provisioning configuration error, got %v",
						err,
					)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected configuration error: %v", err)
			}
		})
	}
}

func TestStripeConnectProvisioningEnvironmentModes(t *testing.T) {
	cases := []struct {
		name        string
		enabled     string
		environment string
		key         string
		wantErr     bool
	}{
		{"development_test_key", "true", "development", "sk_test_synthetic", false},
		{"test_test_key", "true", "test", "sk_test_synthetic", false},
		{"production_live_key", "true", "production", "sk_live_synthetic", false},
		{"development_rejects_live", "true", "development", "sk_live_synthetic", true},
		{"test_rejects_live", "true", "test", "sk_live_synthetic", true},
		{"production_rejects_test", "true", "production", "sk_test_synthetic", true},
		{"unknown_environment", "true", "staging", "sk_test_synthetic", true},
		{"disabled_without_environment", "false", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStripeConnectProvisioningEnvironment(
				StripeConnectProvisioningConfig{EnabledRaw: tc.enabled},
				StripeConfig{SecretKey: tc.key},
				tc.environment,
			)

			if tc.wantErr {
				if !errors.Is(err, ErrStripeConnectProvisioningConfiguration) {
					t.Fatalf("expected configuration error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected environment error: %v", err)
			}
		})
	}
}
