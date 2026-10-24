package main

import (
	"context"
	"strings"
	"testing"
	"time"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recoveryBuilderFakeAuthorizationAdapter struct{}

func (recoveryBuilderFakeAuthorizationAdapter) RetrieveFlexAuthorization(
	ctx context.Context,
	req stripepayment.FlexAuthorizationRequest,
) (*stripepayment.FlexAuthorizationResult, error) {
	return nil, nil
}

func TestAuthorizationRecoveryBuilderRejectsMissingStripeKey(
	t *testing.T,
) {
	for _, key := range []string{"", " ", "\t"} {
		deps, err := newFlexAuthorizationRecoveryDependencies(key)
		if err == nil || deps != nil {
			t.Fatalf("accepted empty Stripe key %q", key)
		}
	}
}

func TestAuthorizationRecoveryBuilderConstructsStripeAdapter(
	t *testing.T,
) {
	deps, err := newFlexAuthorizationRecoveryDependencies(
		"sk_test_configuration_only",
	)
	if err != nil {
		t.Fatal(err)
	}

	if deps == nil || deps.Authorization == nil {
		t.Fatal("Stripe authorization adapter not constructed")
	}
}

func TestAuthorizationRecoveryBuilderRejectsMissingDatabase(
	t *testing.T,
) {
	deps := &flexAuthorizationRecoveryDependencies{
		Authorization: recoveryBuilderFakeAuthorizationAdapter{},
	}

	lifecycle, err := buildFlexAuthorizationRecoveryLifecycle(
		nil,
		deps,
		time.Minute,
		25,
	)

	if err == nil || lifecycle != nil {
		t.Fatal("accepted missing database")
	}
}

func TestAuthorizationRecoveryBuilderRejectsMissingDependencies(
	t *testing.T,
) {
	db := &pgxpool.Pool{}

	for _, deps := range []*flexAuthorizationRecoveryDependencies{
		nil,
		{},
	} {
		lifecycle, err := buildFlexAuthorizationRecoveryLifecycle(
			db,
			deps,
			time.Minute,
			25,
		)

		if err == nil || lifecycle != nil {
			t.Fatal("accepted missing authorization adapter")
		}
	}
}

func TestAuthorizationRecoveryBuilderRejectsInvalidStaleThreshold(
	t *testing.T,
) {
	db := &pgxpool.Pool{}

	deps := &flexAuthorizationRecoveryDependencies{
		Authorization: recoveryBuilderFakeAuthorizationAdapter{},
	}

	for _, threshold := range []time.Duration{
		0,
		-time.Second,
	} {
		lifecycle, err := buildFlexAuthorizationRecoveryLifecycle(
			db,
			deps,
			threshold,
			25,
		)

		if err == nil || lifecycle != nil {
			t.Fatalf(
				"accepted invalid stale threshold %s",
				threshold,
			)
		}
	}
}

func TestAuthorizationRecoveryBuilderAssemblesLifecycle(
	t *testing.T,
) {
	// This zero-value pool is a construction-only sentinel.
	// No database methods may be invoked by the builder.
	db := &pgxpool.Pool{}

	deps := &flexAuthorizationRecoveryDependencies{
		Authorization: recoveryBuilderFakeAuthorizationAdapter{},
	}

	lifecycle, err := buildFlexAuthorizationRecoveryLifecycle(
		db,
		deps,
		time.Minute,
		25,
	)
	if err != nil {
		t.Fatal(err)
	}

	if lifecycle == nil {
		t.Fatal("recovery lifecycle was not constructed")
	}
}

func TestAuthorizationRecoveryBuilderDoesNotStartLifecycle(
	t *testing.T,
) {
	db := &pgxpool.Pool{}

	deps := &flexAuthorizationRecoveryDependencies{
		Authorization: recoveryBuilderFakeAuthorizationAdapter{},
	}

	lifecycle, err := buildFlexAuthorizationRecoveryLifecycle(
		db,
		deps,
		time.Minute,
		25,
	)
	if err != nil {
		t.Fatal(err)
	}

	// A newly assembled lifecycle must not have started.
	err = lifecycle.Wait(context.Background())

	if err == nil ||
		!strings.Contains(err.Error(), "has not started") {
		t.Fatalf(
			"builder unexpectedly started lifecycle: %v",
			err,
		)
	}
}
