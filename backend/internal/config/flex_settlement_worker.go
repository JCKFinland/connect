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

	if enabled && strings.TrimSpace(cfg.Stripe.SecretKey) == "" {
		return fmt.Errorf(
			"%w: STRIPE_SECRET_KEY required when worker enabled",
			ErrInvalidFlexSettlementWorkerConfiguration,
		)
	}

	return nil
}
