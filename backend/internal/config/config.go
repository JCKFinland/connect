package config

import (
	"time"
)

// AppConfig contains application configuration.
type AppConfig struct {
	Name string
	Env  string
	Port string
}

// DatabaseConfig contains database configuration.
type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// JWTConfig contains JWT configuration.
type JWTConfig struct {
	Secret               string
	Issuer               string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
}

// LogConfig contains logging configuration.
type LogConfig struct {
	Level string
}

// Config represents the application's configuration.
type Config struct {
	App         AppConfig
	Database    DatabaseConfig
	JWT         JWTConfig
	Presence    PresenceConfig
	RideRequest RideRequestConfig
	Log         LogConfig
}

// PresenceConfig contains real-time driver presence configuration.
type PresenceConfig struct {
	HeartbeatTimeout time.Duration
}

// RideRequestConfig contains ride-request lifecycle configuration.
type RideRequestConfig struct {
	DefaultMatchingLifetime time.Duration
}

// Load loads the application configuration.
// Load loads the application configuration.
func Load() (*Config, error) {

	if err := LoadEnv(); err != nil {
		return nil, err
	}

	accessDuration, err := time.ParseDuration(
		GetEnv("JWT_ACCESS_TOKEN_DURATION", "15m"),
	)
	if err != nil {
		return nil, err
	}

	refreshDuration, err := time.ParseDuration(
		GetEnv("JWT_REFRESH_TOKEN_DURATION", "720h"),
	)
	if err != nil {
		return nil, err
	}

	heartbeatTimeout, err := time.ParseDuration(
		GetEnv("PRESENCE_HEARTBEAT_TIMEOUT", "2m"),
	)
	if err != nil {
		return nil, err
	}

	defaultMatchingLifetime, err := time.ParseDuration(
		GetEnv(
			"RIDE_REQUEST_DEFAULT_MATCHING_LIFETIME",
			"10m",
		),
	)
	if err != nil {
		return nil, err
	}

	cfg := &Config{

		App: AppConfig{
			Name: GetEnv("APP_NAME", "CONNECT"),
			Env:  GetEnv("APP_ENV", "development"),
			Port: GetEnv("APP_PORT", "8000"),
		},

		Database: DatabaseConfig{
			Host:     GetEnv("DB_HOST", "localhost"),
			Port:     GetEnv("DB_PORT", "5432"),
			Name:     GetEnv("DB_NAME", "connect"),
			User:     GetEnv("DB_USER", "postgres"),
			Password: GetEnv("DB_PASSWORD", ""),
			SSLMode:  GetEnv("DB_SSLMODE", "disable"),
		},

		JWT: JWTConfig{
			Secret:               GetEnv("JWT_SECRET", ""),
			Issuer:               GetEnv("JWT_ISSUER", "connect-api"),
			AccessTokenDuration:  accessDuration,
			RefreshTokenDuration: refreshDuration,
		},

		Presence: PresenceConfig{
			HeartbeatTimeout: heartbeatTimeout,
		},

		RideRequest: RideRequestConfig{
			DefaultMatchingLifetime: defaultMatchingLifetime,
		},

		Log: LogConfig{
			Level: GetEnv("LOG_LEVEL", "info"),
		},
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
