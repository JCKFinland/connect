package config

import (
	"errors"
	"testing"
)

func TestFlexSettlementWorkerDisabledByDefault(t *testing.T) {
	cfg := FlexSettlementWorkerConfig{
		EnabledRaw: "false",
	}

	enabled, err := cfg.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	if enabled {
		t.Fatal("worker unexpectedly enabled")
	}
}

func TestFlexSettlementWorkerZeroValueDisabled(t *testing.T) {
	var cfg FlexSettlementWorkerConfig

	enabled, err := cfg.Enabled()
	if err != nil {
		t.Fatalf("zero-value configuration rejected: %v", err)
	}

	if enabled {
		t.Fatal("zero-value configuration enabled worker")
	}

	if err := ValidateFlexSettlementWorkerConfiguration(
		&Config{},
	); err != nil {
		t.Fatalf("zero-value configuration rejected: %v", err)
	}
}

func TestFlexSettlementWorkerExplicitEnable(t *testing.T) {
	cfg := FlexSettlementWorkerConfig{
		EnabledRaw: "true",
	}

	enabled, err := cfg.Enabled()
	if err != nil {
		t.Fatal(err)
	}

	if !enabled {
		t.Fatal("explicit activation ignored")
	}
}

func TestFlexSettlementWorkerRejectsInvalidActivation(
	t *testing.T,
) {
	for _, raw := range []string{
		"enabled",
		"yes",
		"sometimes",
	} {
		cfg := FlexSettlementWorkerConfig{
			EnabledRaw: raw,
		}

		_, err := cfg.Enabled()

		if !errors.Is(
			err,
			ErrInvalidFlexSettlementWorkerConfiguration,
		) {
			t.Fatalf(
				"activation %q unexpectedly accepted: %v",
				raw,
				err,
			)
		}
	}
}

func TestFlexSettlementWorkerRequiresStripeKey(t *testing.T) {
	cfg := &Config{
		FlexSettlementWorker: FlexSettlementWorkerConfig{
			EnabledRaw: "true",
		},
	}

	err := ValidateFlexSettlementWorkerConfiguration(cfg)

	if !errors.Is(
		err,
		ErrInvalidFlexSettlementWorkerConfiguration,
	) {
		t.Fatalf("missing Stripe key accepted: %v", err)
	}
}

func TestFlexSettlementWorkerAllowsConfiguredStripeKey(
	t *testing.T,
) {
	cfg := &Config{
		FlexSettlementWorker: FlexSettlementWorkerConfig{
			EnabledRaw: "true",
		},
	}

	cfg.Stripe.SecretKey = "sk_test_configuration_only"

	if err := ValidateFlexSettlementWorkerConfiguration(
		cfg,
	); err != nil {
		t.Fatal(err)
	}
}

func TestFlexSettlementWorkerDisabledWithoutStripeKey(
	t *testing.T,
) {
	cfg := &Config{
		FlexSettlementWorker: FlexSettlementWorkerConfig{
			EnabledRaw: "false",
		},
	}

	if err := ValidateFlexSettlementWorkerConfiguration(
		cfg,
	); err != nil {
		t.Fatal(err)
	}
}
