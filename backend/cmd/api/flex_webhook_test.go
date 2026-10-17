package main

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/config"
)

func TestBuildFlexWebhookHandler(t *testing.T) {
	// A non-nil pool pointer is sufficient for construction.
	// These tests do not execute database queries.
	db := &pgxpool.Pool{}

	tests := []struct {
		name        string
		db          *pgxpool.Pool
		config      config.StripeConfig
		wantHandler bool
		wantError   bool
	}{
		{
			name: "FLEX disabled with no Stripe configuration",
		},
		{
			name: "FLEX disabled with existing Stripe API key",
			config: config.StripeConfig{
				SecretKey: "sk_test_example",
			},
		},
		{
			name: "FLEX disabled with existing callback only",
			config: config.StripeConfig{
				WebhookSecret: "whsec_existing",
			},
		},
		{
			name: "FLEX rejects missing Stripe API key",
			config: config.StripeConfig{
				FlexWebhookSecret: "whsec_flex",
			},
			wantError: true,
		},
		{
			name: "FLEX rejects missing database",
			config: config.StripeConfig{
				SecretKey:         "sk_test_example",
				FlexWebhookSecret: "whsec_flex",
			},
			wantError: true,
		},
		{
			name: "FLEX initializes with complete configuration",
			db:   db,
			config: config.StripeConfig{
				SecretKey:         "sk_test_example",
				FlexWebhookSecret: "whsec_flex",
			},
			wantHandler: true,
		},
		{
			name: "FLEX remains independent of existing callback",
			db:   db,
			config: config.StripeConfig{
				SecretKey:         "sk_test_example",
				WebhookSecret:     "whsec_existing",
				FlexWebhookSecret: "whsec_flex",
			},
			wantHandler: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := buildFlexWebhookHandler(
				tt.db,
				tt.config,
			)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected initialization error")
				}

				if handler != nil {
					t.Fatal("handler must be nil on error")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected initialization error: %v", err)
			}

			if (handler != nil) != tt.wantHandler {
				t.Fatalf(
					"unexpected handler state: got %v, want %v",
					handler != nil,
					tt.wantHandler,
				)
			}
		})
	}
}

func TestBuildFlexWebhookHandlerRejectsInvalidConfiguration(
	t *testing.T,
) {
	handler, err := buildFlexWebhookHandler(
		nil,
		config.StripeConfig{
			FlexWebhookSecret: "whsec_flex",
		},
	)

	if !errors.Is(
		err,
		config.ErrInvalidStripeFlexConfiguration,
	) {
		t.Fatalf(
			"expected FLEX configuration error, got %v",
			err,
		)
	}

	if handler != nil {
		t.Fatal("invalid configuration must not create a handler")
	}
}
