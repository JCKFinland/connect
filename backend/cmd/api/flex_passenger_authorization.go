package main

import (
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildPassengerFlexAuthorizationHandler enables payment authorization
// only when FLEX webhook reconciliation has been initialized.
//
// A nil result intentionally disables the passenger endpoint.
func buildPassengerFlexAuthorizationHandler(
	db *pgxpool.Pool,
	stripeConfig config.StripeConfig,
	webhookHandler *api.FlexAuthorizationWebhookHandler,
) (*api.PassengerFlexAuthorizationHandler, error) {
	if err := config.ValidateStripeFlexConfiguration(
		stripeConfig,
	); err != nil {
		return nil, err
	}

	if strings.TrimSpace(stripeConfig.FlexWebhookSecret) == "" {
		return nil, nil
	}

	if webhookHandler == nil {
		return nil, fmt.Errorf(
			"passenger FLEX authorization requires initialized webhook reconciliation",
		)
	}

	if db == nil {
		return nil, fmt.Errorf(
			"passenger FLEX authorization requires a database connection",
		)
	}

	adapter, err := stripepayment.NewFlexAuthorizationAdapter(
		stripeConfig.SecretKey,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize passenger FLEX Stripe adapter: %w",
			err,
		)
	}

	service := flexauthorization.NewService(
		flexauthorization.Dependencies{
			DB:      db,
			Adapter: adapter,
		},
	)

	return api.NewPassengerFlexAuthorizationHandler(
		service,
	), nil
}
