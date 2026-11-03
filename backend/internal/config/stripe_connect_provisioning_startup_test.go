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
