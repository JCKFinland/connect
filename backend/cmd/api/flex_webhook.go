package main

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
)

// buildFlexWebhookHandler constructs the FLEX webhook dependencies.
//
// A nil handler means the webhook route must remain disabled.
// An error means application startup must be rejected.
func buildFlexWebhookHandler(
	db *pgxpool.Pool,
	stripeConfig config.StripeConfig,
) (*api.FlexAuthorizationWebhookHandler, error) {
	if err := config.ValidateStripeFlexConfiguration(stripeConfig); err != nil {
		return nil, err
	}

	if strings.TrimSpace(stripeConfig.FlexWebhookSecret) == "" {
		return nil, nil
	}

	if db == nil {
		return nil, fmt.Errorf(
			"FLEX webhook requires a database connection",
		)
	}

	adapter, err := stripepayment.NewFlexAuthorizationAdapter(
		stripeConfig.SecretKey,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX Stripe adapter: %w",
			err,
		)
	}

	verifier, err := stripepayment.NewFlexWebhookVerifier(
		stripeConfig.FlexWebhookSecret,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize FLEX Stripe verifier: %w",
			err,
		)
	}

	service := flexauthorization.NewService(
		flexauthorization.Dependencies{
			DB:      db,
			Adapter: adapter,
		},
	)

	return api.NewFlexAuthorizationWebhookHandler(
		verifier,
		service,
	), nil
}
