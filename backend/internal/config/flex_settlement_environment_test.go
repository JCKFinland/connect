package config

import (
	"errors"
	"testing"
)

func TestFlexSettlementWorkerEnvironmentSafety(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		database string
		key      string
		wantErr  bool
	}{
		{
			name:     "development test key",
			env:      "development",
			database: "connect",
			key:      "sk_test_configuration_only",
		},
		{
			name:     "test environment test key",
			env:      "test",
			database: "connect_settlement_test",
			key:      "sk_test_configuration_only",
		},
		{
			name:     "production live key",
			env:      "production",
			database: "connect",
			key:      "sk_live_configuration_only",
		},
		{
			name:     "development rejects live key",
			env:      "development",
			database: "connect",
			key:      "sk_live_configuration_only",
			wantErr:  true,
		},
		{
			name:     "test rejects live key",
			env:      "test",
			database: "connect_settlement_test",
			key:      "sk_live_configuration_only",
			wantErr:  true,
		},
		{
			name:     "production rejects test key",
			env:      "production",
			database: "connect",
			key:      "sk_test_configuration_only",
			wantErr:  true,
		},
		{
			name:     "test database rejects development environment",
			env:      "development",
			database: "connect_settlement_test",
			key:      "sk_test_configuration_only",
			wantErr:  true,
		},
		{
			name:     "test database rejects production environment",
			env:      "production",
			database: "connect_settlement_test",
			key:      "sk_live_configuration_only",
			wantErr:  true,
		},
		{
			name:     "rejects unknown key prefix",
			env:      "test",
			database: "connect_settlement_test",
			key:      "invalid_key",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				App: AppConfig{
					Env: tt.env,
				},
				Database: DatabaseConfig{
					Name: tt.database,
				},
				Stripe: StripeConfig{
					SecretKey: tt.key,
				},
				FlexSettlementWorker: FlexSettlementWorkerConfig{
					EnabledRaw: "true",
				},
			}

			err := ValidateFlexSettlementWorkerConfiguration(cfg)

			if tt.wantErr {
				if !errors.Is(
					err,
					ErrInvalidFlexSettlementWorkerConfiguration,
				) {
					t.Fatalf(
						"expected worker configuration error, got %v",
						err,
					)
				}
				return
			}

			if err != nil {
				t.Fatalf(
					"unexpected worker configuration error: %v",
					err,
				)
			}
		})
	}
}
