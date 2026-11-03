package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const stripeProvisioningStartupHelper = "CONNECT_STRIPE_PROVISIONING_STARTUP_HELPER"

// TestStripeProvisioningStartupReachesDatabaseAfterValidation exercises the actual
// application startup function, not only its activation helper.
//
// The child receives an isolated .env and a deliberately unreachable database
// endpoint. The expected error identifies the isolated database boundary.
func TestStripeProvisioningStartupReachesDatabaseAfterValidation(t *testing.T) {
	if os.Getenv(stripeProvisioningStartupHelper) == "1" {
		os.Exit(run())
	}

	directory := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(directory, ".env"),
		[]byte("# isolated startup regression\n"),
		0600,
	); err != nil {
		t.Fatalf("create isolated environment file: %v", err)
	}

	command := exec.Command(
		os.Args[0],
		"-test.run=^TestStripeProvisioningStartupReachesDatabaseAfterValidation$",
	)
	command.Dir = directory

	// Each required setting is explicitly supplied. In particular, the
	// synthetic Stripe key must pass provisioning configuration validation
	// so validated activation proceeds to database initialization.
	overrides := map[string]string{
		stripeProvisioningStartupHelper:          "1",
		"APP_NAME":                               "CONNECT",
		"APP_ENV":                                "test",
		"APP_PORT":                               "8000",
		"JWT_SECRET":                             "connect-startup-regression-synthetic-secret-123456",
		"JWT_ACCESS_TOKEN_DURATION":              "15m",
		"JWT_REFRESH_TOKEN_DURATION":             "720h",
		"BOOKING_COMPANY_ID":                     "startup-regression-synthetic-company",
		"LOG_LEVEL":                              "info",
		"PRESENCE_HEARTBEAT_TIMEOUT":             "2m",
		"RIDE_REQUEST_DEFAULT_MATCHING_LIFETIME": "10m",
		"DOCUMENT_UPLOAD_MAX_BYTES":              "10485760",
		"ROUTING_BASE_URL":                       "http://127.0.0.1:1",
		"DB_HOST":                                "127.0.0.1",
		"DB_PORT":                                "1",
		"DB_NAME":                                "connect_startup_regression_nonexistent",
		"DB_USER":                                "startup_regression",
		"DB_PASSWORD":                            "",
		"DB_SSLMODE":                             "disable",
		"STRIPE_CONNECT_PROVISIONING_ENABLED":    "true",
		"STRIPE_SECRET_KEY":                      "sk_test_connect_startup_synthetic_only",
		"STRIPE_WEBHOOK_SECRET":                  "",
		"STRIPE_FLEX_WEBHOOK_SECRET":             "",
		"FLEX_SETTLEMENT_WORKER_ENABLED":         "false",
		"FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED": "false",
		"CONNECT_REQUIRE_SETTLEMENT_TEST_DB":         "0",
		"PGCONNECT_TIMEOUT":                          "1",
	}

	// Remove inherited versions of every override, including differently
	// cased keys on Windows, before supplying deterministic child settings.
	command.Env = make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			skip := false
			for override := range overrides {
				if strings.EqualFold(key, override) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		command.Env = append(command.Env, entry)
	}
	for key, value := range overrides {
		command.Env = append(command.Env, key+"="+value)
	}

	output, err := command.CombinedOutput()

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("expected startup rejection, got %v; output=%s", err, output)
	}

	if exitError.ExitCode() != 1 {
		t.Fatalf(
			"expected startup exit code 1, got %d; output=%s",
			exitError.ExitCode(), output,
		)
	}

	message := string(output)

	if !strings.Contains(
		message,
		"Starting CONNECT Backend",
	) {
		t.Fatalf(
			"startup did not pass activation validation; output=%s",
			message,
		)
	}

	if !strings.Contains(
		message,
		"Database connection failed",
	) {
		t.Fatalf(
			"startup did not reach isolated database boundary; output=%s",
			message,
		)
	}

	for _, unexpected := range []string{
		"Configuration error:",
		"FLEX Stripe configuration error:",
		"Stripe Connect provisioning configuration error:",
		"Stripe provisioning activation unavailable:",
		"Migration failed",
		"Stripe provisioning schema not ready",
		"production route assembly is not wired",
	} {
		if strings.Contains(message, unexpected) {
			t.Fatalf(
				"startup crossed an unexpected boundary (%s): %s",
				unexpected, message,
			)
		}
	}
}
