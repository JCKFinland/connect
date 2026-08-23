package config

import (
	"fmt"
	"strconv"
)

func Validate(cfg *Config) error {

	if cfg.App.Name == "" {
		return fmt.Errorf("APP_NAME is required")
	}

	switch cfg.App.Env {

	case "development", "test", "production":

	default:
		return fmt.Errorf(
			"invalid APP_ENV '%s' (allowed: development, test, production)",
			cfg.App.Env,
		)
	}

	port, err := strconv.Atoi(cfg.App.Port)

	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid APP_PORT")
	}

	if cfg.Database.Host == "" {
		return fmt.Errorf("DB_HOST is required")
	}

	if cfg.Database.Name == "" {
		return fmt.Errorf("DB_NAME is required")
	}

	if cfg.Database.User == "" {
		return fmt.Errorf("DB_USER is required")
	}

	if cfg.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}

	if len(cfg.JWT.Secret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}

	if cfg.Log.Level == "" {
		return fmt.Errorf("LOG_LEVEL is required")
	}

	if cfg.RideRequest.DefaultMatchingLifetime <= 0 {
		return fmt.Errorf(
			"ride request default matching lifetime must be greater than zero",
		)
	}

	return nil
}
