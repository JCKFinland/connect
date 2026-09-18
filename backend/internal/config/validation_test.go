package config

import (
	"strings"
	"testing"
	"time"
)

func validTestConfig() *Config {
	return &Config{
		App: AppConfig{
			Name: "CONNECT",
			Env:  "test",
			Port: "8000",
		},
		Database: DatabaseConfig{
			Host: "localhost",
			Name: "connect",
			User: "postgres",
		},
		JWT: JWTConfig{
			Secret: strings.Repeat("x", 32),
		},
		RideRequest: RideRequestConfig{
			DefaultMatchingLifetime: 10 * time.Minute,
		},
		Booking: BookingConfig{
			CompanyID: "345c5e3e-b07a-4e16-837d-e5d32254d6f3",
		},
		Log: LogConfig{
			Level: "info",
		},
	}
}

func TestValidateAcceptsBookingCompanyID(t *testing.T) {
	cfg := validTestConfig()

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateRejectsMissingBookingCompanyID(t *testing.T) {
	cfg := validTestConfig()
	cfg.Booking.CompanyID = ""

	err := Validate(cfg)
	if err == nil {
		t.Fatal("Validate() expected error")
	}

	if err.Error() != "BOOKING_COMPANY_ID is required" {
		t.Fatalf(
			"Validate() error = %q, want %q",
			err.Error(),
			"BOOKING_COMPANY_ID is required",
		)
	}
}
