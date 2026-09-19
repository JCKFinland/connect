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

// StripeConfig contains Stripe payment-provider configuration.
type StripeConfig struct {
	SecretKey     string
	WebhookSecret string
}

// Config represents the application's configuration.
type Config struct {
	App         AppConfig
	Database    DatabaseConfig
	JWT         JWTConfig
	Presence    PresenceConfig
	RideRequest RideRequestConfig
	Booking     BookingConfig
	Routing     RoutingConfig
	Stripe      StripeConfig
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

// BookingConfig contains customer booking configuration.
type BookingConfig struct {
	CompanyID string
}

// RoutingConfig contains road-routing provider configuration.
type RoutingConfig struct {
	BaseURL string
}

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

		Booking: BookingConfig{
			CompanyID: GetEnv(
				"BOOKING_COMPANY_ID",
				"",
			),
		},

		Routing: RoutingConfig{
			BaseURL: GetEnv(
				"ROUTING_BASE_URL",
				"https://router.project-osrm.org",
			),
		},

		Stripe: StripeConfig{
			SecretKey: GetEnv(
				"STRIPE_SECRET_KEY",
				"",
			),

			WebhookSecret: GetEnv(
				"STRIPE_WEBHOOK_SECRET",
				"",
			),
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
