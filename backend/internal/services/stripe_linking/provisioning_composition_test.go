package stripelinking_test

import (
	"errors"
	"testing"

	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

func TestStripeProvisioningCompositionDisabledByDefault(t *testing.T) {
	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		false,
		stripelinking.ProvisioningCoordinatorDeps{},
		nil,
	)

	if coordinator != nil {
		t.Fatal("disabled composition returned a coordinator")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected disabled error: %v", err)
	}
}

func TestStripeProvisioningCompositionRejectsMissingDependencies(t *testing.T) {
	coordinator, err := stripelinking.NewProvisioningCoordinatorIfEnabled(
		true,
		stripelinking.ProvisioningCoordinatorDeps{},
		nil,
	)

	if coordinator != nil {
		t.Fatal("incomplete composition returned a coordinator")
	}

	if !errors.Is(
		err,
		stripelinking.ErrStripeProvisioningCompositionUnavailable,
	) {
		t.Fatalf("unexpected incomplete error: %v", err)
	}
}
