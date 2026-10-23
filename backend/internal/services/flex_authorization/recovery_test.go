package flex_authorization

import (
	"context"
	"errors"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	stripego "github.com/stripe/stripe-go/v86"
)

func setupKnownProviderRecoveryTest(t *testing.T) (
	*authorizationServiceFixture,
	string,
	string,
) {
	t.Helper()

	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx,
		f.DB,
		authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	providerID := "pi_flex_readonly_recovery_" + authorization.ID

	_, err = postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultProcessing,
	)
	if err != nil {
		t.Fatal(err)
	}

	return f, authorization.ID, providerID
}

func TestFlexAuthorizationRecoveryReadOnly(t *testing.T) {
	f, authorizationID, providerID :=
		setupKnownProviderRecoveryTest(t)

	ctx := context.Background()

	f.adapter.result = &stripepayment.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	service := NewRecoveryService(f.DB, f.adapter)

	var beforeStatus string
	var beforeProviderID string

	err := f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorizationID).Scan(
		&beforeStatus,
		&beforeProviderID,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Recover(ctx, authorizationID)
	if err != nil {
		t.Fatal(err)
	}

	if result == nil ||
		result.AuthorizationID != authorizationID ||
		result.ProviderAuthorizationID != providerID ||
		result.ProviderStatus !=
			stripego.PaymentIntentStatusRequiresCapture ||
		!result.Authorized {
		t.Fatalf("unexpected recovery result: %+v", result)
	}

	f.adapter.mu.Lock()

	if f.adapter.calls != 1 ||
		len(f.adapter.requests) != 1 ||
		f.adapter.requests[0].ProviderAuthorizationID != providerID {
		f.adapter.mu.Unlock()
		t.Fatal("recovery did not enforce known-provider retrieval")
	}

	f.adapter.mu.Unlock()

	var afterStatus string
	var afterProviderID string

	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorizationID).Scan(
		&afterStatus,
		&afterProviderID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if afterStatus != beforeStatus ||
		afterProviderID != beforeProviderID {
		t.Fatal("read-only recovery modified authorization state")
	}
}

func TestFlexAuthorizationRecoveryRejectsProviderMismatch(t *testing.T) {
	f, authorizationID, _ :=
		setupKnownProviderRecoveryTest(t)

	f.adapter.result = &stripepayment.FlexAuthorizationResult{
		ProviderAuthorizationID: "pi_wrong_provider",
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	service := NewRecoveryService(f.DB, f.adapter)

	result, err := service.Recover(
		context.Background(),
		authorizationID,
	)

	if err == nil || result != nil {
		t.Fatalf(
			"expected provider identity rejection: result=%+v err=%v",
			result,
			err,
		)
	}
}

func TestFlexAuthorizationRecoveryRejectsInvalidProviderFlags(t *testing.T) {
	f, authorizationID, providerID :=
		setupKnownProviderRecoveryTest(t)

	f.adapter.result = &stripepayment.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              false,
	}

	service := NewRecoveryService(f.DB, f.adapter)

	result, err := service.Recover(
		context.Background(),
		authorizationID,
	)

	if err == nil || result != nil {
		t.Fatalf(
			"expected invalid provider flag rejection: result=%+v err=%v",
			result,
			err,
		)
	}
}

func TestFlexAuthorizationRecoveryRejectsUnknownProvider(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx,
		f.DB,
		authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	service := NewRecoveryService(f.DB, f.adapter)

	result, err := service.Recover(ctx, authorization.ID)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationNotFound,
	) || result != nil {
		t.Fatalf(
			"expected unknown-provider rejection: result=%+v err=%v",
			result,
			err,
		)
	}

	if f.adapter.callCount() != 0 {
		t.Fatal("unknown-provider recovery contacted Stripe adapter")
	}
}

func TestFlexAuthorizationRecoveryRejectsTerminalState(t *testing.T) {
	f, authorizationID, providerID :=
		setupKnownProviderRecoveryTest(t)

	_, err := postgresrepo.ReconcileFlexAuthorization(
		context.Background(),
		f.DB,
		authorizationID,
		providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatal(err)
	}

	service := NewRecoveryService(f.DB, f.adapter)

	result, err := service.Recover(
		context.Background(),
		authorizationID,
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationNotFound,
	) || result != nil {
		t.Fatalf(
			"expected terminal rejection: result=%+v err=%v",
			result,
			err,
		)
	}

	if f.adapter.callCount() != 0 {
		t.Fatal("terminal recovery contacted Stripe adapter")
	}
}

func TestFlexAuthorizationRecoveryRejectsMissingDependencies(t *testing.T) {
	var service *RecoveryService

	if _, err := service.Recover(
		context.Background(),
		"invalid",
	); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("expected unavailable service, got %v", err)
	}

	service = NewRecoveryService(nil, nil)

	if _, err := service.Recover(
		context.Background(),
		"invalid",
	); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("expected unavailable dependencies, got %v", err)
	}
}
