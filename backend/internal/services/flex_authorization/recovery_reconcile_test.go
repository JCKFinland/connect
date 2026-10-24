package flex_authorization

import (
	"context"
	"errors"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	stripego "github.com/stripe/stripe-go/v86"
)

func setupRecoveryReconcileFixture(t *testing.T) (
	*authorizationServiceFixture,
	string,
	string,
) {
	t.Helper()

	return setupKnownProviderRecoveryTest(t)
}

type recoveryReconcileAdapter struct {
	execute func(
		context.Context,
		stripepayment.FlexAuthorizationRequest,
	) (*stripepayment.FlexAuthorizationResult, error)
}

func (a recoveryReconcileAdapter) ExecuteFlexAuthorization(
	ctx context.Context,
	request stripepayment.FlexAuthorizationRequest,
) (*stripepayment.FlexAuthorizationResult, error) {
	return a.execute(ctx, request)
}

func TestRecoverAndReconcileAuthorized(t *testing.T) {
	f, authorizationID, providerID :=
		setupRecoveryReconcileFixture(t)

	adapter := recoveryReconcileAdapter{
		execute: func(
			ctx context.Context,
			request stripepayment.FlexAuthorizationRequest,
		) (*stripepayment.FlexAuthorizationResult, error) {
			if request.ProviderAuthorizationID != providerID {
				t.Fatalf(
					"recovery attempted unknown provider: %q",
					request.ProviderAuthorizationID,
				)
			}

			return &stripepayment.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              true,
			}, nil
		},
	}

	service := NewRecoveryService(f.DB, adapter)

	result, err := service.RecoverAndReconcile(
		context.Background(),
		authorizationID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result == nil ||
		result.Status != "AUTHORIZED" ||
		!result.Authorized ||
		result.ProviderAuthorizationID != providerID {
		t.Fatalf("unexpected authorized recovery: %+v", result)
	}
}

func TestRecoverAndReconcileProcessing(t *testing.T) {
	f, authorizationID, providerID :=
		setupRecoveryReconcileFixture(t)

	adapter := recoveryReconcileAdapter{
		execute: func(
			ctx context.Context,
			request stripepayment.FlexAuthorizationRequest,
		) (*stripepayment.FlexAuthorizationResult, error) {
			return &stripepayment.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusProcessing,
			}, nil
		},
	}

	service := NewRecoveryService(f.DB, adapter)

	result, err := service.RecoverAndReconcile(
		context.Background(),
		authorizationID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result == nil ||
		result.Status != "PROCESSING" ||
		result.Authorized {
		t.Fatalf("unexpected processing recovery: %+v", result)
	}
}

func TestRecoverAndReconcileCancelled(t *testing.T) {
	f, authorizationID, providerID :=
		setupRecoveryReconcileFixture(t)

	adapter := recoveryReconcileAdapter{
		execute: func(
			ctx context.Context,
			request stripepayment.FlexAuthorizationRequest,
		) (*stripepayment.FlexAuthorizationResult, error) {
			return &stripepayment.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusCanceled,
			}, nil
		},
	}

	service := NewRecoveryService(f.DB, adapter)

	result, err := service.RecoverAndReconcile(
		context.Background(),
		authorizationID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result == nil || result.Status != "CANCELLED" {
		t.Fatalf("unexpected cancelled recovery: %+v", result)
	}
}

func TestRecoverAndReconcileRejectsProviderMismatch(t *testing.T) {
	f, authorizationID, _ :=
		setupRecoveryReconcileFixture(t)

	adapter := recoveryReconcileAdapter{
		execute: func(
			ctx context.Context,
			request stripepayment.FlexAuthorizationRequest,
		) (*stripepayment.FlexAuthorizationResult, error) {
			return &stripepayment.FlexAuthorizationResult{
				ProviderAuthorizationID: "pi_different",
				ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
				Authorized:              true,
			}, nil
		},
	}

	service := NewRecoveryService(f.DB, adapter)

	_, err := service.RecoverAndReconcile(
		context.Background(),
		authorizationID,
	)

	if err == nil {
		t.Fatal("expected provider mismatch rejection")
	}

	var status string
	err = f.DB.QueryRow(context.Background(), `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, authorizationID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" {
		t.Fatalf("provider mismatch changed state: %s", status)
	}
}

func TestRecoverAndReconcileRejectsConcurrentWebhook(t *testing.T) {
	f, authorizationID, providerID :=
		setupRecoveryReconcileFixture(t)

	adapter := recoveryReconcileAdapter{
		execute: func(
			ctx context.Context,
			request stripepayment.FlexAuthorizationRequest,
		) (*stripepayment.FlexAuthorizationResult, error) {
			// Simulate a verified webhook committing while
			// the recovery provider retrieval is in flight.
			_, err := postgresrepo.ReconcileFlexAuthorization(
				ctx,
				f.DB,
				authorizationID,
				providerID,
				postgresrepo.FlexResultAuthorized,
			)
			if err != nil {
				return nil, err
			}

			// This provider result is now stale.
			return &stripepayment.FlexAuthorizationResult{
				ProviderAuthorizationID: providerID,
				ProviderStatus:          stripego.PaymentIntentStatusProcessing,
			}, nil
		},
	}

	service := NewRecoveryService(f.DB, adapter)

	_, err := service.RecoverAndReconcile(
		context.Background(),
		authorizationID,
	)

	if !errors.Is(
		err,
		postgresrepo.ErrFlexAuthorizationResultConflict,
	) {
		t.Fatalf("expected stale recovery conflict, got %v", err)
	}

	var status string

	err = f.DB.QueryRow(context.Background(), `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, authorizationID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "AUTHORIZED" {
		t.Fatalf(
			"stale recovery overwrote webhook result: %s",
			status,
		)
	}
}
