package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripeProvisioningStartupRejectsCredentialModeMismatch(
	t *testing.T,
) {
	if os.Getenv(stripeProvisioningStartupHelper) == "credential-mode" {
		os.Exit(run())
	}

	cases := []struct {
		name        string
		environment string
		secretKey   string
		expected    string
	}{
		{
			name:        "test_rejects_live_key",
			environment: "test",
			secretKey:   "sk_live_connect_synthetic_only",
			expected:    "non-production provisioning requires Stripe test-mode secret key",
		},
		{
			name:        "development_rejects_live_key",
			environment: "development",
			secretKey:   "sk_live_connect_synthetic_only",
			expected:    "non-production provisioning requires Stripe test-mode secret key",
		},
		{
			name:        "production_rejects_test_key",
			environment: "production",
			secretKey:   "sk_test_connect_synthetic_only",
			expected:    "production provisioning requires Stripe live-mode secret key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()

			if err := os.WriteFile(
				filepath.Join(directory, ".env"),
				[]byte("# isolated credential-mode startup test\n"),
				0600,
			); err != nil {
				t.Fatalf("create isolated .env: %v", err)
			}

			command := exec.Command(
				os.Args[0],
				"-test.run=^TestStripeProvisioningStartupRejectsCredentialModeMismatch$",
			)
			command.Dir = directory

			overrides := map[string]string{
				stripeProvisioningStartupHelper:          "credential-mode",
				"APP_NAME":                               "CONNECT",
				"APP_ENV":                                tc.environment,
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
				"STRIPE_SECRET_KEY":                      tc.secretKey,
				"STRIPE_WEBHOOK_SECRET":                  "",
				"STRIPE_FLEX_WEBHOOK_SECRET":             "",
				"FLEX_SETTLEMENT_WORKER_ENABLED":         "false",
				"FLEX_AUTHORIZATION_RECOVERY_WORKER_ENABLED": "false",
				"CONNECT_REQUIRE_SETTLEMENT_TEST_DB":         "0",
				"PGCONNECT_TIMEOUT":                          "1",
			}

			command.Env = make(
				[]string, 0, len(os.Environ())+len(overrides),
			)

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
				command.Env = append(
					command.Env, key+"="+value,
				)
			}

			output, err := command.CombinedOutput()

			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				t.Fatalf(
					"expected startup rejection, got %v; output=%s",
					err, output,
				)
			}

			if exitError.ExitCode() != 1 {
				t.Fatalf(
					"expected exit code 1, got %d; output=%s",
					exitError.ExitCode(), output,
				)
			}

			message := string(output)

			if !strings.Contains(
				message,
				"Stripe Connect provisioning configuration error:",
			) {
				t.Fatalf(
					"expected provisioning configuration rejection; output=%s",
					message,
				)
			}

			if !strings.Contains(message, tc.expected) {
				t.Fatalf(
					"expected credential-mode rejection %q; output=%s",
					tc.expected, message,
				)
			}

			for _, unexpected := range []string{
				"Starting CONNECT Backend",
				"Database connection failed",
				"Migration failed",
				"Stripe provisioning schema not ready",
			} {
				if strings.Contains(message, unexpected) {
					t.Fatalf(
						"startup crossed forbidden boundary %q; output=%s",
						unexpected, message,
					)
				}
			}
		})
	}
}
