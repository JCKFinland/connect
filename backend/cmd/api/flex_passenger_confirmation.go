package main

import (
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	flexoffer "github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildPassengerFlexConfirmationHandler shares the existing
// authorization activation guard. Confirmation must never
// be exposed without the configured authorization pathway.
func buildPassengerFlexConfirmationHandler(
	db *pgxpool.Pool,
	cfg *config.Config,
	authorization *api.PassengerFlexAuthorizationHandler,
) (*api.PassengerFlexConfirmationHandler, error) {
	if authorization == nil {
		return nil, nil
	}

	if db == nil {
		return nil, fmt.Errorf(
			"passenger FLEX confirmation requires a database connection",
		)
	}

	if cfg == nil {
		return nil, fmt.Errorf(
			"passenger FLEX confirmation requires application configuration",
		)
	}

	service := flexoffer.NewService(flexoffer.Dependencies{
		DB:     db,
		Config: cfg,
	})

	return api.NewPassengerFlexConfirmationHandler(service), nil
}
