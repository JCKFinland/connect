package flex_authorization

import (
	"context"
	"errors"
	"testing"

	flexoffer "github.com/JCKFinland/connect/backend/internal/services/flex_offer"
)

func TestFlexUncertainPaymentCannotCreateTrip(t *testing.T) {
	f := setupAuthorizationServiceTest(t)
	ctx := context.Background()

	confirmation := flexoffer.NewService(
		flexoffer.Dependencies{
			DB:     f.DB,
			Config: f.Config,
		},
	)

	f.adapter.err = errors.New(
		"provider connection interrupted after authorization request",
	)

	first, err := f.service.Execute(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)
	if err == nil {
		t.Fatalf("expected ambiguous provider error, result=%+v", first)
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf(
			"expected one provider attempt, got %d",
			f.adapter.callCount(),
		)
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
		t.Fatalf("read authorization after provider error: %v", err)
	}

	if authorizationID == "" ||
		authorizationStatus != "PROCESSING" ||
		idempotencyKey == "" {
		t.Fatalf(
			"unsafe authorization state: id=%s status=%s key=%s",
			authorizationID,
			authorizationStatus,
			idempotencyKey,
		)
	}

	trip, confirmErr := confirmation.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)

	if !errors.Is(
		confirmErr,
		flexoffer.ErrFlexConfirmationNotEligible,
	) || trip != nil {
		t.Fatalf(
			"trip confirmation bypassed payment authorization: trip=%+v err=%v",
			trip,
			confirmErr,
		)
	}

	retry, retryErr := f.service.Execute(
		ctx,
		f.RideID,
		f.AuctionID,
		f.passengerID,
	)

	if !errors.Is(retryErr, ErrAuthorizationPending) {
		t.Fatalf(
			"expected pending recovery, result=%+v err=%v",
			retry,
			retryErr,
		)
	}

	if f.adapter.callCount() != 1 {
		t.Fatalf(
			"retry contacted provider again: calls=%d",
			f.adapter.callCount(),
		)
	}

	var persistedID string
	var persistedStatus string
	var persistedKey string
	var authorizationCount int

	err = f.DB.QueryRow(ctx, `
        SELECT id, status, idempotency_key
        FROM flex_authorizations
        WHERE auction_id = $1
    `, f.AuctionID).Scan(
		&persistedID,
		&persistedStatus,
		&persistedKey,
	)
	if err != nil {
		t.Fatalf("read authorization after retry: %v", err)
	}

	err = f.DB.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_authorizations
        WHERE auction_id = $1
    `, f.AuctionID).Scan(&authorizationCount)
	if err != nil {
		t.Fatalf("count authorizations: %v", err)
	}

	if authorizationCount != 1 ||
		persistedID != authorizationID ||
		persistedStatus != "PROCESSING" ||
		persistedKey != idempotencyKey {
		t.Fatalf(
			"authorization identity changed: count=%d id=%s status=%s key=%s",
			authorizationCount,
			persistedID,
			persistedStatus,
			persistedKey,
		)
	}

	var tripCount int

	err = f.DB.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM trips
        WHERE ride_request_id = $1
    `, f.RideID).Scan(&tripCount)
	if err != nil {
		t.Fatalf("count trips: %v", err)
	}

	if tripCount != 0 {
		t.Fatalf(
			"ambiguous payment created %d trips",
			tripCount,
		)
	}
}
