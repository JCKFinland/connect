package config

import (
	"errors"
	"testing"
)

func TestFlexAuthorizationRecoveryWorkerDisabledByDefault(
	t *testing.T,
) {
	var worker FlexAuthorizationRecoveryWorkerConfig

	enabled, err := worker.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	if enabled {
		t.Fatal("zero-value recovery worker unexpectedly enabled")
	}

	if err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(
		&Config{},
	); err != nil {
		t.Fatalf("disabled worker unexpectedly rejected: %v", err)
	}
}

func TestFlexAuthorizationRecoveryWorkerExplicitActivation(
	t *testing.T,
) {
	cases := []struct {
		raw     string
		enabled bool
	}{
		{"", false},
		{"false", false},
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"0", false},
		{" true ", true},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			worker := FlexAuthorizationRecoveryWorkerConfig{
				EnabledRaw: tc.raw,
			}

			enabled, err := worker.Enabled()
			if err != nil {
				t.Fatal(err)
			}

			if enabled != tc.enabled {
				t.Fatalf(
					"activation %q: got %v, want %v",
					tc.raw,
					enabled,
					tc.enabled,
				)
			}
		})
	}
}

func TestFlexAuthorizationRecoveryWorkerRejectsInvalidActivation(
	t *testing.T,
) {
	for _, raw := range []string{
		"enabled",
		"yes",
		"on",
		"sometimes",
		"2",
	} {
		t.Run(raw, func(t *testing.T) {
			cfg := &Config{
				FlexAuthorizationRecoveryWorker: FlexAuthorizationRecoveryWorkerConfig{
					EnabledRaw: raw,
				},
			}

			err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(cfg)

			if !errors.Is(
				err,
				ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
			) {
				t.Fatalf(
					"invalid activation %q accepted: %v",
					raw,
					err,
				)
			}
		})
	}
}

func TestFlexAuthorizationRecoveryWorkerRequiresStripeKey(
	t *testing.T,
) {
	cfg := &Config{
		FlexAuthorizationRecoveryWorker: FlexAuthorizationRecoveryWorkerConfig{
			EnabledRaw: "true",
		},
	}

	err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(cfg)

	if !errors.Is(
		err,
		ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
	) {
		t.Fatalf("missing Stripe key accepted: %v", err)
	}
}

func TestFlexAuthorizationRecoveryWorkerEnforcesStripeEnvironment(
	t *testing.T,
) {
	cases := []struct {
		name       string
		env        string
		key        string
		shouldPass bool
	}{
		{
			name:       "development test key",
			env:        "development",
			key:        "sk_test_configuration_only",
			shouldPass: true,
		},
		{
			name:       "test environment test key",
			env:        "test",
			key:        "sk_test_configuration_only",
			shouldPass: true,
		},
		{
			name:       "production live key",
			env:        "production",
			key:        "sk_live_configuration_only",
			shouldPass: true,
		},
		{
			name: "development rejects live key",
			env:  "development",
			key:  "sk_live_configuration_only",
		},
		{
			name: "test rejects live key",
			env:  "test",
			key:  "sk_live_configuration_only",
		},
		{
			name: "production rejects test key",
			env:  "production",
			key:  "sk_test_configuration_only",
		},
		{
			name: "unsupported environment",
			env:  "staging",
			key:  "sk_test_configuration_only",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				App: AppConfig{
					Env: tc.env,
				},
				Stripe: StripeConfig{
					SecretKey: tc.key,
				},
				FlexAuthorizationRecoveryWorker: FlexAuthorizationRecoveryWorkerConfig{
					EnabledRaw: "true",
				},
			}

			err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(cfg)

			if tc.shouldPass {
				if err != nil {
					t.Fatalf("valid configuration rejected: %v", err)
				}
				return
			}

			if !errors.Is(
				err,
				ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
			) {
				t.Fatalf("unsafe configuration accepted: %v", err)
			}
		})
	}
}

func TestFlexAuthorizationRecoveryWorkerProtectsSettlementTestDatabase(
	t *testing.T,
) {
	for _, env := range []string{
		"development",
		"production",
	} {
		t.Run(env, func(t *testing.T) {
			key := "sk_test_configuration_only"
			if env == "production" {
				key = "sk_live_configuration_only"
			}

			cfg := &Config{
				App: AppConfig{
					Env: env,
				},
				Database: DatabaseConfig{
					Name: "connect_settlement_test",
				},
				Stripe: StripeConfig{
					SecretKey: key,
				},
				FlexAuthorizationRecoveryWorker: FlexAuthorizationRecoveryWorkerConfig{
					EnabledRaw: "true",
				},
			}

			err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(cfg)

			if !errors.Is(
				err,
				ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
			) {
				t.Fatalf(
					"unsafe test database/environment combination accepted: %v",
					err,
				)
			}
		})
	}

	cfg := &Config{
		App: AppConfig{
			Env: "test",
		},
		Database: DatabaseConfig{
			Name: "connect_settlement_test",
		},
		Stripe: StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
		FlexAuthorizationRecoveryWorker: FlexAuthorizationRecoveryWorkerConfig{
			EnabledRaw: "true",
		},
	}

	if err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(
		cfg,
	); err != nil {
		t.Fatalf("valid test environment rejected: %v", err)
	}
}

func TestFlexAuthorizationRecoveryWorkerIndependentFromSettlement(
	t *testing.T,
) {
	cfg := &Config{
		App: AppConfig{
			Env: "test",
		},
		Stripe: StripeConfig{
			SecretKey: "sk_test_configuration_only",
		},
		FlexSettlementWorker: FlexSettlementWorkerConfig{
			EnabledRaw: "false",
		},
		FlexAuthorizationRecoveryWorker: FlexAuthorizationRecoveryWorkerConfig{
			EnabledRaw: "true",
		},
	}

	if err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(
		cfg,
	); err != nil {
		t.Fatal(err)
	}

	settlementEnabled, err := cfg.FlexSettlementWorker.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	recoveryEnabled, err := cfg.FlexAuthorizationRecoveryWorker.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	if settlementEnabled || !recoveryEnabled {
		t.Fatal("recovery activation unexpectedly changed settlement activation")
	}

	cfg.FlexSettlementWorker.EnabledRaw = "true"
	cfg.FlexAuthorizationRecoveryWorker.EnabledRaw = "false"

	settlementEnabled, err = cfg.FlexSettlementWorker.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	recoveryEnabled, err = cfg.FlexAuthorizationRecoveryWorker.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	if !settlementEnabled || recoveryEnabled {
		t.Fatal("settlement activation unexpectedly changed recovery activation")
	}
}

func TestFlexAuthorizationRecoveryWorkerRejectsNilConfiguration(
	t *testing.T,
) {
	err := ValidateFlexAuthorizationRecoveryWorkerConfiguration(nil)

	if !errors.Is(
		err,
		ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
	) {
		t.Fatalf("nil configuration accepted: %v", err)
	}
}
