package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration = errors.New(
	"invalid FLEX authorization recovery worker configuration",
)

// FlexAuthorizationRecoveryWorkerConfig controls background
// verification of existing Stripe authorizations.
//
// The zero value is disabled. This setting is independent
// of FLEX settlement discovery and capture processing.
type FlexAuthorizationRecoveryWorkerConfig struct {
	EnabledRaw string
}

func (c FlexAuthorizationRecoveryWorkerConfig) Enabled() (bool, error) {
	raw := strings.TrimSpace(c.EnabledRaw)

	if raw == "" {
		return false, nil
	}

	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf(
			"%w: invalid activation value",
			ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
		)
	}

	return enabled, nil
}

// ValidateFlexAuthorizationRecoveryWorkerConfiguration
// validates the independently controlled recovery worker.
//
// Disabled recovery requires no Stripe credentials.
// Enabled recovery requires environment-appropriate credentials.
func ValidateFlexAuthorizationRecoveryWorkerConfiguration(
	cfg *Config,
) error {
	if cfg == nil {
		return ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration
	}

	enabled, err := cfg.FlexAuthorizationRecoveryWorker.Enabled()
	if err != nil {
		return err
	}

	if !enabled {
		return nil
	}

	key := strings.TrimSpace(cfg.Stripe.SecretKey)

	if key == "" {
		return fmt.Errorf(
			"%w: STRIPE_SECRET_KEY required when recovery enabled",
			ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
		)
	}

	switch cfg.App.Env {
	case "development", "test":
		if !strings.HasPrefix(key, "sk_test_") {
			return fmt.Errorf(
				"%w: non-production recovery requires Stripe test-mode secret key",
				ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
			)
		}

	case "production":
		if !strings.HasPrefix(key, "sk_live_") {
			return fmt.Errorf(
				"%w: production recovery requires Stripe live-mode secret key",
				ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
			)
		}

	default:
		return fmt.Errorf(
			"%w: unsupported application environment",
			ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
		)
	}

	if cfg.Database.Name == "connect_settlement_test" &&
		cfg.App.Env != "test" {
		return fmt.Errorf(
			"%w: settlement test database requires APP_ENV=test",
			ErrInvalidFlexAuthorizationRecoveryWorkerConfiguration,
		)
	}

	return nil
}
