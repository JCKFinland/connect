package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const stripeProductionActivationStartupHelper = "CONNECT_STRIPE_PRODUCTION_ACTIVATION_STARTUP_HELPER"

func TestStripeProvisioningProductionActivationRejectedBeforeDatabase(t *testing.T) {
	if os.Getenv(stripeProductionActivationStartupHelper) == "1" {
		os.Exit(run())
	}

	directory := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(directory, ".env"),
		[]byte("# isolated production activation regression\n"),
		0600,
	); err != nil {
		t.Fatalf("create isolated environment: %v", err)
	}

	command := exec.Command(
		os.Args[0],
		"-test.run=^TestStripeProvisioningProductionActivationRejectedBeforeDatabase$",
	)
	command.Dir = directory

	overrides := map[string]string{
		stripeProductionActivationStartupHelper:  "1",
		"APP_NAME":                               "CONNECT",
		"APP_ENV":                                "production",
		"APP_PORT":                               "8000",
		"JWT_SECRET":                             "connect-production-activation-synthetic-secret-123456",
		"JWT_ACCESS_TOKEN_DURATION":              "15m",
		"JWT_REFRESH_TOKEN_DURATION":             "720h",
		"BOOKING_COMPANY_ID":                     "production-activation-synthetic-company",
		"LOG_LEVEL":                              "info",
		"PRESENCE_HEARTBEAT_TIMEOUT":             "2m",
		"RIDE_REQUEST_DEFAULT_MATCHING_LIFETIME": "10m",
		"DOCUMENT_UPLOAD_MAX_BYTES":              "10485760",
		"ROUTING_BASE_URL":                       "http://127.0.0.1:1",
		"DB_HOST":                                "127.0.0.1",
		"DB_PORT":                                "1",
		"DB_NAME":                                "connect_production_activation_nonexistent",
		"DB_USER":                                "production_activation_regression",
		"DB_PASSWORD":                            "",
		"DB_SSLMODE":                             "disable",
		"STRIPE_CONNECT_PROVISIONING_ENABLED":    "true",
		"STRIPE_SECRET_KEY":                      "sk_live_connect_synthetic_only",
		"STRIPE_WEBHOOK_SECRET":                  "",
		"STRIPE_FLEX_WEBHOOK_SECRET":             "",
		"CONNECT_STRIPE_PROVISIONING_PRODUCTION_AUTHORIZED": "",
		"FLEX_SETTLEMENT_WORKER_ENABLED":                    "false",
		"FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED":        "false",
		"CONNECT_REQUIRE_SETTLEMENT_TEST_DB":                "0",
		"PGCONNECT_TIMEOUT":                                 "1",
	}

	command.Env = make([]string, 0, len(os.Environ())+len(overrides))

	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}

		skip := false
		for override := range overrides {
			if strings.EqualFold(key, override) {
				skip = true
				break
			}
		}

		if !skip {
			command.Env = append(command.Env, entry)
		}
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
		"Stripe provisioning activation unavailable:",
	) {
		t.Fatalf("activation rejection missing: %s", message)
	}

	if !strings.Contains(
		message,
		"production Stripe provisioning activation is not authorized",
	) {
		t.Fatalf("authorization rejection missing: %s", message)
	}

	for _, unexpected := range []string{
		"Starting CONNECT Backend",
		"Database connection failed",
		"Migration failed",
		"Stripe provisioning schema not ready",
		"production route assembly is not wired",
	} {
		if strings.Contains(message, unexpected) {
			t.Fatalf(
				"startup crossed forbidden boundary %q: %s",
				unexpected, message,
			)
		}
	}
}
