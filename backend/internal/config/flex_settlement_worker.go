package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidFlexSettlementWorkerConfiguration = errors.New(
	"invalid FLEX settlement worker configuration",
)

// FlexSettlementWorkerConfig keeps financial background
// execution disabled unless explicitly enabled.
type FlexSettlementWorkerConfig struct {
	EnabledRaw string
}

// Enabled rejects malformed values instead of silently
// enabling or disabling financial processing.
func (c FlexSettlementWorkerConfig) Enabled() (bool, error) {
	raw := strings.TrimSpace(c.EnabledRaw)

	if raw == "" {
		return false, nil
	}

	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf(
			"%w: invalid activation value",
			ErrInvalidFlexSettlementWorkerConfiguration,
		)
	}

	return enabled, nil
}

func ValidateFlexSettlementWorkerConfiguration(
	cfg *Config,
) error {
	if cfg == nil {
		return ErrInvalidFlexSettlementWorkerConfiguration
	}

	enabled, err := cfg.FlexSettlementWorker.Enabled()
	if err != nil {
		return err
	}

	if !enabled {
		return nil
	}

	key := strings.TrimSpace(cfg.Stripe.SecretKey)
	if key == "" {
		return fmt.Errorf(
			"%w: STRIPE_SECRET_KEY required when worker enabled",
			ErrInvalidFlexSettlementWorkerConfiguration,
		)
	}

	switch cfg.App.Env {
	case "development", "test":
		if !strings.HasPrefix(key, "sk_test_") {
			return fmt.Errorf(
				"%w: non-production worker requires Stripe test-mode secret key",
				ErrInvalidFlexSettlementWorkerConfiguration,
			)
		}
	case "production":
		if !strings.HasPrefix(key, "sk_live_") {
			return fmt.Errorf(
				"%w: production worker requires Stripe live-mode secret key",
				ErrInvalidFlexSettlementWorkerConfiguration,
			)
		}
	default:
		return fmt.Errorf(
			"%w: unsupported application environment",
			ErrInvalidFlexSettlementWorkerConfiguration,
		)
	}

	if cfg.Database.Name == "connect_settlement_test" &&
		cfg.App.Env != "test" {
		return fmt.Errorf(
			"%w: settlement test database requires APP_ENV=test",
			ErrInvalidFlexSettlementWorkerConfiguration,
		)
	}

	return nil
}
