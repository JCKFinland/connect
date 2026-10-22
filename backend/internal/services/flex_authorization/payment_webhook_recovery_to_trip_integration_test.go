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

func TestFlexInterruptedPaymentWebhookRecoveryToTrip(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

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
                SELECT id FROM flex_authorizations
                WHERE auction_id = $1
            )
        `, f.AuctionID); err != nil {
			t.Errorf("cleanup webhook events: %v", err)
		}

		if _, err := f.DB.Exec(cleanupCtx, `
            DELETE FROM flex_authorizations
            WHERE auction_id = $1
        `, f.AuctionID); err != nil {
			t.Errorf("cleanup authorization: %v", err)
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
			t.Errorf("cleanup auction: %v", err)
		}
	})

	confirmation := flexoffer.NewService(
		flexoffer.Dependencies{
			DB:     f.DB,
			Config: f.Config,
		},
	)

	f.adapter.err = errors.New(
		"Stripe connection interrupted after PaymentIntent creation",
	)

	_, err := f.service.Execute(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err == nil {
		t.Fatal("expected interrupted provider request")
	}

	var authorizationID string
	var authorizationStatus string
	var idempotencyKey string

	err = f.DB.QueryRow(ctx, `
        SELECT id, status, idempotency_key
        FROM flex_authorizations
        WHERE auction_id = $1
    `, f.AuctionID).Scan(
		&authorizationID,
		&authorizationStatus,
		&idempotencyKey,
	)
	if err != nil {
		t.Fatalf("read interrupted authorization: %v", err)
	}

	if authorizationStatus != "PROCESSING" {
		t.Fatalf(
			"expected PROCESSING, got %s",
			authorizationStatus,
		)
	}

	prematureTrip, prematureErr := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if prematureTrip != nil ||
		!errors.Is(
			prematureErr,
			flexoffer.ErrFlexConfirmationNotEligible,
		) {
		t.Fatalf(
			"premature trip confirmation: trip=%+v err=%v",
			prematureTrip,
			prematureErr,
		)
	}

	providerID := "pi_flex_webhook_e2e_" + uuid.NewString()

	f.adapter.mu.Lock()
	f.adapter.err = nil
	f.adapter.result = &stripe.FlexAuthorizationResult{
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		Authorized:              true,
	}
	f.adapter.mu.Unlock()

	var offerID string
	var amountCents int64

	err = f.DB.QueryRow(ctx, `
        SELECT offer_id, ROUND(amount * 100)::bigint
        FROM flex_authorizations
        WHERE id = $1
    `, authorizationID).Scan(&offerID, &amountCents)
	if err != nil {
		t.Fatalf("read authorization financial identity: %v", err)
	}

	event := &stripe.VerifiedFlexAuthorizationEvent{
		EventID:                 "evt_flex_webhook_e2e_" + uuid.NewString(),
		EventType:               "payment_intent.amount_capturable_updated",
		ProviderAuthorizationID: providerID,
		ProviderStatus:          stripego.PaymentIntentStatusRequiresCapture,
		AuthorizationID:         authorizationID,
		AuctionID:               f.AuctionID,
		OfferID:                 offerID,
		PassengerID:             f.passengerID,
		AmountCents:             amountCents,
		Currency:                "EUR",
		CaptureMethod:           stripego.PaymentIntentCaptureMethodManual,
	}

	recovered, err := f.service.ReconcileWebhook(ctx, event)
	if err != nil {
		t.Fatalf("recover authorization through webhook: %v", err)
	}

	if recovered == nil ||
		recovered.ID != authorizationID ||
		recovered.Status != "AUTHORIZED" ||
		recovered.ProviderAuthorizationID == nil ||
		*recovered.ProviderAuthorizationID != providerID ||
		recovered.IdempotencyKey != idempotencyKey {
		t.Fatalf(
			"unexpected recovered authorization: %+v",
			recovered,
		)
	}

	if f.adapter.callCount() != 2 {
		t.Fatalf(
			"expected one interrupted create and one recovery retrieval, got %d",
			f.adapter.callCount(),
		)
	}

	f.adapter.mu.Lock()
	requests := append(
		[]stripe.FlexAuthorizationRequest(nil),
		f.adapter.requests...,
	)
	f.adapter.mu.Unlock()

	if len(requests) != 2 ||
		requests[0].ProviderAuthorizationID != "" ||
		requests[1].ProviderAuthorizationID != providerID ||
		requests[0].IdempotencyKey != idempotencyKey ||
		requests[1].IdempotencyKey != idempotencyKey {
		t.Fatalf("recovery did not preserve provider identity")
	}

	trip, err := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err != nil {
		t.Fatalf("confirm recovered FLEX ride: %v", err)
	}

	if trip == nil ||
		trip.RideRequestID != f.RideID ||
		trip.DriverID != f.DriverID ||
		trip.Status != "ASSIGNED" {
		t.Fatalf("unexpected recovered trip: %+v", trip)
	}

	var tripCount int
	err = f.DB.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM trips
        WHERE ride_request_id = $1
    `, f.RideID).Scan(&tripCount)
	if err != nil {
		t.Fatalf("count recovered trips: %v", err)
	}

	if tripCount != 1 {
		t.Fatalf("expected exactly one trip, got %d", tripCount)
	}

	duplicateTrip, duplicateErr := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if duplicateTrip != nil ||
		!errors.Is(
			duplicateErr,
			flexoffer.ErrFlexConfirmationNotEligible,
		) {
		t.Fatalf(
			"duplicate confirmation succeeded: trip=%+v err=%v",
			duplicateTrip,
			duplicateErr,
		)
	}
}
