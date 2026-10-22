package main

import (
	"testing"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
)

func TestBuildPassengerFlexConfirmationHandler(t *testing.T) {
	t.Run("disabled without authorization", func(t *testing.T) {
		handler, err := buildPassengerFlexConfirmationHandler(
			nil, nil, nil,
		)
		if err != nil || handler != nil {
			t.Fatalf("expected disabled confirmation, got %v, %v",
				handler, err)
		}
	})

	t.Run("rejects missing database", func(t *testing.T) {
		authorization := api.NewPassengerFlexAuthorizationHandler(nil)

		handler, err := buildPassengerFlexConfirmationHandler(
			nil,
			&config.Config{},
			authorization,
		)
		if err == nil || handler != nil {
			t.Fatalf("expected database validation failure")
		}
	})
}
