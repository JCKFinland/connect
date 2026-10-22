package main

import (
	"testing"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuildPassengerFlexAuthorizationHandler(t *testing.T) {
	tests := []struct {
		name         string
		config       config.StripeConfig
		webhookReady bool
		dbReady      bool
		wantEnabled  bool
		wantError    bool
	}{
		{
			name: "disabled without Stripe configuration",
		},
		{
			name: "disabled with Stripe API key only",
			config: config.StripeConfig{
				SecretKey: "sk_test_placeholder",
			},
		},
		{
			name: "disabled with unrelated payment callback",
			config: config.StripeConfig{
				SecretKey:     "sk_test_placeholder",
				WebhookSecret: "whsec_existing",
			},
		},
		{
			name: "reject missing Stripe API key",
			config: config.StripeConfig{
				FlexWebhookSecret: "whsec_flex",
			},
			wantError: true,
		},
		{
			name: "reject missing webhook initialization",
			config: config.StripeConfig{
				SecretKey:         "sk_test_placeholder",
				FlexWebhookSecret: "whsec_flex",
			},
			wantError: true,
		},
		{
			name: "reject missing database",
			config: config.StripeConfig{
				SecretKey:         "sk_test_placeholder",
				FlexWebhookSecret: "whsec_flex",
			},
			webhookReady: true,
			wantError:    true,
		},
		{
			name: "enable with complete configuration",
			config: config.StripeConfig{
				SecretKey:         "sk_test_placeholder",
				FlexWebhookSecret: "whsec_flex",
			},
			webhookReady: true,
			dbReady:      true,
			wantEnabled:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var db *pgxpool.Pool
			if tc.dbReady {
				// Constructor checks only dependency presence.
				// No database connection or query is performed.
				db = &pgxpool.Pool{}
			}

			var webhook *api.FlexAuthorizationWebhookHandler
			if tc.webhookReady {
				webhook = &api.FlexAuthorizationWebhookHandler{}
			}

			handler, err :=
				buildPassengerFlexAuthorizationHandler(
					db,
					tc.config,
					webhook,
				)

			if (err != nil) != tc.wantError {
				t.Fatalf(
					"expected error=%t, got %v",
					tc.wantError,
					err,
				)
			}

			if (handler != nil) != tc.wantEnabled {
				t.Fatalf(
					"expected enabled=%t, got enabled=%t",
					tc.wantEnabled,
					handler != nil,
				)
			}
		})
	}
}
