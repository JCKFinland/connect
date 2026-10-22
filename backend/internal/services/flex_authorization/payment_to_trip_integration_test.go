package flex_authorization

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	flexoffer "github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/google/uuid"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexPaymentToTripIntegration(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	// Run before the shared fixture cleanup (LIFO order).
	// Confirmed trips must be removed before referenced records.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := f.DB.Exec(cleanupCtx, `
            DELETE FROM trips
            WHERE ride_request_id = $1
        `, f.RideID); err != nil {
			t.Errorf("cleanup confirmed trip: %v", err)
		}

		if _, err := f.DB.Exec(cleanupCtx, `
            DELETE FROM flex_webhook_events
            WHERE authorization_id IN (
                SELECT id
                FROM flex_authorizations
                WHERE auction_id = $1
            )
        `, f.AuctionID); err != nil {
			t.Errorf("cleanup FLEX webhook events: %v", err)
		}

		if _, err := f.DB.Exec(cleanupCtx, `
            DELETE FROM flex_authorizations
            WHERE auction_id = $1
        `, f.AuctionID); err != nil {
			t.Errorf("cleanup FLEX authorization: %v", err)
		}

		if _, err := f.DB.Exec(cleanupCtx, `
            UPDATE flex_auctions
            SET status = 'OPEN',
                selected_offer_id = NULL,
                resolved_at = NULL,
                updated_at = clock_timestamp()
            WHERE id = $1
              AND status IN (
                  'PENDING_AUTHORIZATION',
                  'CONFIRMED'
              )
        `, f.AuctionID); err != nil {
			t.Errorf("release selected FLEX offer: %v", err)
		}
	})

	confirmation := flexoffer.NewService(
		flexoffer.Dependencies{
			DB:     f.DB,
			Config: f.Config,
		},
	)

	// A selected offer is not sufficient to create a trip.
	prematureTrip, prematureErr := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)

	if !errors.Is(
		prematureErr,
		flexoffer.ErrFlexConfirmationNotEligible,
	) {
		t.Fatalf(
			"expected pre-authorization rejection, trip=%+v err=%v",
			prematureTrip,
			prematureErr,
		)
	}

	if prematureTrip != nil {
		t.Fatalf(
			"trip created before authorization: %+v",
			prematureTrip,
		)
	}

	providerID := "pi_flex_e2e_" + uuid.NewString()

	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}

	authorized, err := f.service.Execute(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)

	if err != nil {
		t.Fatalf("execute FLEX authorization: %v", err)
	}

	if authorized == nil ||
		!authorized.Authorized ||
		authorized.Status != "AUTHORIZED" ||
		authorized.ProviderAuthorizationID != providerID {
		t.Fatalf(
			"unexpected authorization result: %+v",
			authorized,
		)
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf(
			"expected one fake provider authorization, got %d",
			f.adapter.callCount(),
		)
	}

	trip, err := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)

	if err != nil {
		t.Fatalf("confirm authorized FLEX ride: %v", err)
	}

	if trip == nil ||
		trip.ID == "" ||
		trip.RideRequestID != f.RideID ||
		trip.DriverID != f.DriverID ||
		trip.Status != "ASSIGNED" {
		t.Fatalf("unexpected confirmed trip: %+v", trip)
	}

	var tripCount int

	if err := f.DB.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM trips
        WHERE ride_request_id = $1
    `, f.RideID).Scan(&tripCount); err != nil {
		t.Fatalf("count confirmed trips: %v", err)
	}

	if tripCount != 1 {
		t.Fatalf(
			"expected exactly one confirmed trip, got %d",
			tripCount,
		)
	}

	duplicateTrip, duplicateErr := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)

	if !errors.Is(
		duplicateErr,
		flexoffer.ErrFlexConfirmationNotEligible,
	) || duplicateTrip != nil {
		t.Fatalf(
			"duplicate confirmation unexpectedly succeeded: trip=%+v err=%v",
			duplicateTrip,
			duplicateErr,
		)
	}

	var authorizationStatus string
	var persistedProviderID string

	if err := f.DB.QueryRow(ctx, `
        SELECT status, provider_authorization_id
        FROM flex_authorizations
        WHERE auction_id = $1
    `, f.AuctionID).Scan(
		&authorizationStatus,
		&persistedProviderID,
	); err != nil {
		t.Fatalf("read authorization after confirmation: %v", err)
	}

	if authorizationStatus != "AUTHORIZED" ||
		persistedProviderID != providerID {
		t.Fatalf(
			"confirmation mutated authorization: status=%s provider=%s",
			authorizationStatus,
			persistedProviderID,
		)
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf(
			"confirmation unexpectedly invoked payment provider",
		)
	}
}
