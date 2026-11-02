package config

import (
	"errors"
	"testing"
)

func TestStripeConnectProvisioningFeatureGate(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      bool
		wantError bool
	}{
		{"unset", "", false, false},
		{"disabled", "false", false, false},
		{"enabled", "true", true, false},
		{"uppercase", "TRUE", false, true},
		{"mixedcase", "True", false, true},
		{"numeric", "1", false, true},
		{"whitespace", " true ", false, true},
		{"invalid", "enabled", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (StripeConnectProvisioningConfig{
				EnabledRaw: tc.raw,
			}).Enabled()

			if got != tc.want {
				t.Fatalf("enabled=%v, want %v", got, tc.want)
			}

			if tc.wantError {
				if !errors.Is(
					err,
					ErrStripeConnectProvisioningConfiguration,
				) {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
