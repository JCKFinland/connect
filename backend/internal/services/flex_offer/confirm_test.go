package flex_offer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestConfirmAuthorizedRideSuccess(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	// Register cleanup after the shared fixture so that it runs first.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		// Remove the confirmed trip before the ride request and driver.
		if _, err := f.DB.Exec(cleanupCtx, `
			DELETE FROM trips
			WHERE ride_request_id = $1
		`, f.RideID); err != nil {
			t.Errorf("cleanup confirmed FLEX trip: %v", err)
		}

		// Remove payment authorization before the selected offer.
		if _, err := f.DB.Exec(cleanupCtx, `
			DELETE FROM flex_authorizations
			WHERE auction_id = $1
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup FLEX authorization: %v", err)
		}

		// Release the selected-offer foreign-key reference.
		if _, err := f.DB.Exec(cleanupCtx, `
			UPDATE flex_auctions
			SET status = 'OPEN',
			    selected_offer_id = NULL,
			    resolved_at = NULL,
			    updated_at = clock_timestamp()
			WHERE id = $1
			  AND status IN ('PENDING_AUTHORIZATION', 'CONFIRMED')
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup confirmed FLEX auction: %v", err)
		}
	})

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string
	err = f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID)
	if err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	_, err = service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx,
		f.DB,
		authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim FLEX authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	providerID := "pi_flex_confirm_" + uuid.NewString()

	authorized, err := postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("authorize FLEX payment: %v", err)
	}
	if authorized == nil || authorized.Status != "AUTHORIZED" {
		t.Fatalf("unexpected authorization: %+v", authorized)
	}

	trip, err := service.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		passengerID,
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

	// The confirmed trip must resolve to the exact negotiated
	// amount backed by the authorized Stripe payment.
	contract, err := postgresrepo.ResolveConfirmedFlexFareContract(
		ctx,
		f.DB,
		trip.ID,
	)
	if err != nil {
		t.Fatalf("resolve confirmed FLEX fare contract: %v", err)
	}
	if contract == nil {
		t.Fatal("confirmed FLEX trip has no fare contract")
	}
	if contract.TripID != trip.ID ||
		contract.AmountCents != offer.OfferedAmountCents ||
		contract.Currency != "EUR" {
		t.Fatalf(
			"confirmed FLEX fare contract mismatch: contract=%+v offer=%+v",
			contract,
			offer,
		)
	}

	var auctionProfileID string
	var auctionPricingVersion string
	if err := f.DB.QueryRow(ctx, `
		SELECT pricing_profile_id, pricing_version
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&auctionProfileID,
		&auctionPricingVersion,
	); err != nil {
		t.Fatalf("read frozen FLEX pricing provenance: %v", err)
	}

	if contract.PricingProfileID != auctionProfileID ||
		contract.PricingVersion != auctionPricingVersion {
		t.Fatalf(
			"FLEX pricing provenance mismatch: contract=%+v profile=%s version=%s",
			contract,
			auctionProfileID,
			auctionPricingVersion,
		)
	}

	var tripCount int
	var persistedTripStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*), MAX(status)
		FROM trips
		WHERE ride_request_id = $1
	`, f.RideID).Scan(&tripCount, &persistedTripStatus)
	if err != nil {
		t.Fatalf("read confirmed trip: %v", err)
	}
	if tripCount != 1 || persistedTripStatus != "ASSIGNED" {
		t.Fatalf(
			"unexpected trip persistence: count=%d status=%s",
			tripCount, persistedTripStatus,
		)
	}

	var presenceStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, f.DriverID).Scan(&presenceStatus)
	if err != nil {
		t.Fatalf("read confirmed driver presence: %v", err)
	}
	if presenceStatus != "BUSY" {
		t.Fatalf("expected BUSY driver, got %s", presenceStatus)
	}

	var rideStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&rideStatus)
	if err != nil {
		t.Fatalf("read confirmed ride request: %v", err)
	}
	if rideStatus != "ACCEPTED" {
		t.Fatalf("expected ACCEPTED ride, got %s", rideStatus)
	}

	var offerStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_offers
		WHERE id = $1
	`, offer.ID).Scan(&offerStatus)
	if err != nil {
		t.Fatalf("read confirmed offer: %v", err)
	}
	if offerStatus != "ACCEPTED" {
		t.Fatalf("expected ACCEPTED offer, got %s", offerStatus)
	}

	var auctionStatus string
	var selectedOfferID string
	err = f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&auctionStatus,
		&selectedOfferID,
	)
	if err != nil {
		t.Fatalf("read confirmed auction: %v", err)
	}
	if auctionStatus != "CONFIRMED" ||
		selectedOfferID != offer.ID {
		t.Fatalf(
			"unexpected auction: status=%s selected=%s",
			auctionStatus, selectedOfferID,
		)
	}

	var authorizationStatus string
	var persistedProviderID string
	err = f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(
		&authorizationStatus,
		&persistedProviderID,
	)
	if err != nil {
		t.Fatalf("read confirmed authorization: %v", err)
	}
	if authorizationStatus != "AUTHORIZED" ||
		persistedProviderID != providerID {
		t.Fatalf(
			"authorization unexpectedly changed: status=%s provider=%s",
			authorizationStatus, persistedProviderID,
		)
	}

	// A confirmed FLEX ride must not be assigned a second time.
	duplicateTrip, duplicateErr := service.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if !errors.Is(duplicateErr, ErrFlexConfirmationNotEligible) {
		t.Fatalf(
			"expected duplicate confirmation rejection, got trip=%+v err=%v",
			duplicateTrip, duplicateErr,
		)
	}
	if duplicateTrip != nil {
		t.Fatalf("duplicate confirmation returned a trip: %+v", duplicateTrip)
	}

	var duplicateTripCount int
	var originalTripID string
	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id::text)
		FROM trips
		WHERE ride_request_id = $1
	`, f.RideID).Scan(&duplicateTripCount, &originalTripID)
	if err != nil {
		t.Fatalf("verify duplicate trip prevention: %v", err)
	}
	if duplicateTripCount != 1 || originalTripID != trip.ID {
		t.Fatalf(
			"duplicate confirmation changed trips: count=%d id=%s original=%s",
			duplicateTripCount, originalTripID, trip.ID,
		)
	}

	var finalRideStatus string
	var finalOfferStatus string
	var finalAuctionStatus string
	var finalPresenceStatus string

	for _, check := range []struct {
		query string
		id    string
		dest  *string
	}{
		{
			"SELECT status FROM ride_requests WHERE id = $1",
			f.RideID,
			&finalRideStatus,
		},
		{
			"SELECT status FROM flex_offers WHERE id = $1",
			offer.ID,
			&finalOfferStatus,
		},
		{
			"SELECT status FROM flex_auctions WHERE id = $1",
			f.AuctionID,
			&finalAuctionStatus,
		},
		{
			"SELECT availability_status FROM driver_presence WHERE driver_id = $1",
			f.DriverID,
			&finalPresenceStatus,
		},
	} {
		if err := f.DB.QueryRow(
			ctx, check.query, check.id,
		).Scan(check.dest); err != nil {
			t.Fatalf("verify state after duplicate confirmation: %v", err)
		}
	}

	if finalRideStatus != "ACCEPTED" ||
		finalOfferStatus != "ACCEPTED" ||
		finalAuctionStatus != "CONFIRMED" ||
		finalPresenceStatus != "BUSY" {
		t.Fatalf(
			"duplicate confirmation changed state: ride=%s offer=%s auction=%s presence=%s",
			finalRideStatus,
			finalOfferStatus,
			finalAuctionStatus,
			finalPresenceStatus,
		)
	}
}

// TestConfirmAuthorizedRideRejectsPendingAuthorization verifies that a
// passenger-selected FLEX offer cannot become a trip before payment
// authorization reaches the AUTHORIZED state.
func TestConfirmAuthorizedRideRejectsPendingAuthorization(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	// This cleanup executes before the shared fixture cleanup.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := f.DB.Exec(cleanupCtx, `
			DELETE FROM flex_authorizations
			WHERE auction_id = $1
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup pending FLEX authorization: %v", err)
		}

		if _, err := f.DB.Exec(cleanupCtx, `
			UPDATE flex_auctions
			SET status = 'OPEN',
			    selected_offer_id = NULL,
			    resolved_at = NULL,
			    updated_at = clock_timestamp()
			WHERE id = $1
			  AND status = 'PENDING_AUTHORIZATION'
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup selected FLEX auction: %v", err)
		}
	})

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string
	if err := f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID); err != nil {
		t.Fatalf("resolve FLEX passenger: %v", err)
	}

	if _, err := service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	); err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("create pending FLEX authorization: %v", err)
	}
	if authorization == nil || authorization.Status != "PENDING" {
		t.Fatalf("expected PENDING authorization, got %+v", authorization)
	}

	trip, err := service.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if !errors.Is(err, ErrFlexConfirmationNotEligible) {
		t.Fatalf(
			"expected pending authorization rejection, trip=%+v err=%v",
			trip, err,
		)
	}
	if trip != nil {
		t.Fatalf("pending authorization produced a trip: %+v", trip)
	}

	var tripCount int
	if err := f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trips
		WHERE ride_request_id = $1
	`, f.RideID).Scan(&tripCount); err != nil {
		t.Fatalf("count FLEX trips: %v", err)
	}
	if tripCount != 0 {
		t.Fatalf("pending authorization created %d trips", tripCount)
	}

	var rideStatus string
	var offerStatus string
	var auctionStatus string
	var presenceStatus string
	var authorizationStatus string

	checks := []struct {
		name  string
		query string
		id    string
		dest  *string
	}{
		{
			"ride",
			"SELECT status FROM ride_requests WHERE id = $1",
			f.RideID,
			&rideStatus,
		},
		{
			"offer",
			"SELECT status FROM flex_offers WHERE id = $1",
			offer.ID,
			&offerStatus,
		},
		{
			"auction",
			"SELECT status FROM flex_auctions WHERE id = $1",
			f.AuctionID,
			&auctionStatus,
		},
		{
			"driver presence",
			"SELECT availability_status FROM driver_presence WHERE driver_id = $1",
			f.DriverID,
			&presenceStatus,
		},
		{
			"authorization",
			"SELECT status FROM flex_authorizations WHERE id = $1",
			authorization.ID,
			&authorizationStatus,
		},
	}

	for _, check := range checks {
		if err := f.DB.QueryRow(
			ctx, check.query, check.id,
		).Scan(check.dest); err != nil {
			t.Fatalf("read %s state: %v", check.name, err)
		}
	}

	if rideStatus != "PENDING" ||
		offerStatus != "SELECTED" ||
		auctionStatus != "PENDING_AUTHORIZATION" ||
		presenceStatus != "AVAILABLE" ||
		authorizationStatus != "PENDING" {
		t.Fatalf(
			"pending authorization changed state: ride=%s offer=%s auction=%s presence=%s authorization=%s",
			rideStatus,
			offerStatus,
			auctionStatus,
			presenceStatus,
			authorizationStatus,
		)
	}
}

// TestConfirmAuthorizedRideRejectsOfflineDriver verifies that payment
// authorization does not override the driver's current availability.
func TestConfirmAuthorizedRideRejectsOfflineDriver(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	// Runs before the shared fixture cleanup.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

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
			  AND status = 'PENDING_AUTHORIZATION'
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup selected FLEX auction: %v", err)
		}
	})

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string
	if err := f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID); err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	if _, err := service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	); err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx,
		f.DB,
		authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	providerID := "pi_flex_offline_" + uuid.NewString()

	authorized, err := postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("authorize FLEX payment: %v", err)
	}
	if authorized == nil || authorized.Status != "AUTHORIZED" {
		t.Fatalf("expected AUTHORIZED payment, got %+v", authorized)
	}

	// The driver becomes unavailable after payment authorization.
	result, err := f.DB.Exec(ctx, `
		UPDATE driver_presence
		SET is_online = FALSE,
		    availability_status = 'OFFLINE',
		    updated_at = clock_timestamp()
		WHERE driver_id = $1
		  AND is_online = TRUE
		  AND availability_status = 'AVAILABLE'
	`, f.DriverID)
	if err != nil {
		t.Fatalf("take driver offline: %v", err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf(
			"expected one driver presence update, got %d",
			result.RowsAffected(),
		)
	}

	trip, err := service.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if !errors.Is(err, ErrFlexConfirmationNotEligible) {
		t.Fatalf(
			"expected offline driver rejection, trip=%+v err=%v",
			trip, err,
		)
	}
	if trip != nil {
		t.Fatalf("offline driver confirmation returned trip: %+v", trip)
	}

	var tripCount int
	if err := f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trips
		WHERE ride_request_id = $1
	`, f.RideID).Scan(&tripCount); err != nil {
		t.Fatalf("count trips: %v", err)
	}
	if tripCount != 0 {
		t.Fatalf("offline driver confirmation created %d trips", tripCount)
	}

	var rideStatus string
	var offerStatus string
	var auctionStatus string
	var presenceStatus string
	var authorizationStatus string
	var online bool

	checks := []struct {
		name  string
		query string
		id    string
		dest  any
	}{
		{
			"ride",
			"SELECT status FROM ride_requests WHERE id = $1",
			f.RideID,
			&rideStatus,
		},
		{
			"offer",
			"SELECT status FROM flex_offers WHERE id = $1",
			offer.ID,
			&offerStatus,
		},
		{
			"auction",
			"SELECT status FROM flex_auctions WHERE id = $1",
			f.AuctionID,
			&auctionStatus,
		},
		{
			"driver presence",
			"SELECT availability_status FROM driver_presence WHERE driver_id = $1",
			f.DriverID,
			&presenceStatus,
		},
		{
			"driver online",
			"SELECT is_online FROM driver_presence WHERE driver_id = $1",
			f.DriverID,
			&online,
		},
		{
			"authorization",
			"SELECT status FROM flex_authorizations WHERE id = $1",
			authorization.ID,
			&authorizationStatus,
		},
	}

	for _, check := range checks {
		if err := f.DB.QueryRow(
			ctx, check.query, check.id,
		).Scan(check.dest); err != nil {
			t.Fatalf("read %s: %v", check.name, err)
		}
	}

	if rideStatus != "PENDING" ||
		offerStatus != "SELECTED" ||
		auctionStatus != "PENDING_AUTHORIZATION" ||
		presenceStatus != "OFFLINE" ||
		online ||
		authorizationStatus != "AUTHORIZED" {
		t.Fatalf(
			"unexpected state after offline rejection: ride=%s offer=%s auction=%s presence=%s online=%v authorization=%s",
			rideStatus,
			offerStatus,
			auctionStatus,
			presenceStatus,
			online,
			authorizationStatus,
		)
	}
}

// TestConfirmAuthorizedRideRollsBackAfterTripCreation verifies that a
// database failure after trip insertion and driver claim rolls back
// the complete FLEX confirmation transaction.
func TestConfirmAuthorizedRideRollsBackAfterTripCreation(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string
	if err := f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID); err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	if _, err := service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	); err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx, f.DB, f.RideID, f.AuctionID, passengerID,
	)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim authorization: claimed=%v err=%v", claimed, err)
	}

	providerID := "pi_flex_rollback_" + uuid.NewString()
	authorized, err := postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if authorized == nil || authorized.Status != "AUTHORIZED" {
		t.Fatalf("expected AUTHORIZED payment, got %+v", authorized)
	}

	// Unique database objects prevent collisions with other tests.
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	functionName := "test_flex_rollback_" + suffix
	triggerName := "test_flex_rollback_trigger_" + suffix

	// Restrict the failure to this test's ride request.
	createFunction := fmt.Sprintf(`
		CREATE FUNCTION %s()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $body$
		BEGIN
			IF NEW.id = '%s'::uuid
			   AND OLD.status = 'PENDING'
			   AND NEW.status = 'ACCEPTED' THEN
				RAISE EXCEPTION 'test FLEX confirmation rollback';
			END IF;
			RETURN NEW;
		END;
		$body$
	`, functionName, f.RideID)

	if _, err := f.DB.Exec(ctx, createFunction); err != nil {
		t.Fatalf("create rollback trigger function: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := f.DB.Exec(cleanupCtx,
			fmt.Sprintf(
				"DROP TRIGGER IF EXISTS %s ON ride_requests",
				triggerName,
			),
		); err != nil {
			t.Errorf("drop rollback trigger: %v", err)
		}

		if _, err := f.DB.Exec(cleanupCtx,
			fmt.Sprintf(
				"DROP FUNCTION IF EXISTS %s()",
				functionName,
			),
		); err != nil {
			t.Errorf("drop rollback trigger function: %v", err)
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
			  AND status = 'PENDING_AUTHORIZATION'
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup selected auction: %v", err)
		}
	})

	createTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE UPDATE OF status ON ride_requests
		FOR EACH ROW
		EXECUTE FUNCTION %s()
	`, triggerName, functionName)

	if _, err := f.DB.Exec(ctx, createTrigger); err != nil {
		t.Fatalf("create rollback trigger: %v", err)
	}

	trip, err := service.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if err == nil {
		t.Fatal("expected injected database failure")
	}
	if trip != nil {
		t.Fatalf("failed confirmation returned trip: %+v", trip)
	}
	if !strings.Contains(err.Error(), "test FLEX confirmation rollback") {
		t.Fatalf("unexpected confirmation error: %v", err)
	}

	var tripCount int
	if err := f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trips
		WHERE ride_request_id = $1
	`, f.RideID).Scan(&tripCount); err != nil {
		t.Fatalf("count trips after rollback: %v", err)
	}
	if tripCount != 0 {
		t.Fatalf("rollback left %d trips", tripCount)
	}

	var rideStatus string
	var offerStatus string
	var auctionStatus string
	var presenceStatus string
	var authorizationStatus string

	checks := []struct {
		name  string
		query string
		id    string
		dest  *string
	}{
		{
			"ride",
			"SELECT status FROM ride_requests WHERE id = $1",
			f.RideID,
			&rideStatus,
		},
		{
			"offer",
			"SELECT status FROM flex_offers WHERE id = $1",
			offer.ID,
			&offerStatus,
		},
		{
			"auction",
			"SELECT status FROM flex_auctions WHERE id = $1",
			f.AuctionID,
			&auctionStatus,
		},
		{
			"presence",
			"SELECT availability_status FROM driver_presence WHERE driver_id = $1",
			f.DriverID,
			&presenceStatus,
		},
		{
			"authorization",
			"SELECT status FROM flex_authorizations WHERE id = $1",
			authorization.ID,
			&authorizationStatus,
		},
	}

	for _, check := range checks {
		if err := f.DB.QueryRow(
			ctx, check.query, check.id,
		).Scan(check.dest); err != nil {
			t.Fatalf("read %s after rollback: %v", check.name, err)
		}
	}

	if rideStatus != "PENDING" ||
		offerStatus != "SELECTED" ||
		auctionStatus != "PENDING_AUTHORIZATION" ||
		presenceStatus != "AVAILABLE" ||
		authorizationStatus != "AUTHORIZED" {
		t.Fatalf(
			"rollback left inconsistent state: ride=%s offer=%s auction=%s presence=%s authorization=%s",
			rideStatus,
			offerStatus,
			auctionStatus,
			presenceStatus,
			authorizationStatus,
		)
	}
}

// TestConfirmAuthorizedRideConcurrentSingleWinner verifies that concurrent
// confirmation requests cannot create duplicate trips or claim a driver twice.
func TestConfirmAuthorizedRideConcurrentSingleWinner(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	// This cleanup runs before the shared fixture cleanup.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := f.DB.Exec(cleanupCtx, `
			DELETE FROM trips
			WHERE ride_request_id = $1
		`, f.RideID); err != nil {
			t.Errorf("cleanup confirmed trip: %v", err)
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
			  AND status IN ('PENDING_AUTHORIZATION', 'CONFIRMED')
		`, f.AuctionID); err != nil {
			t.Errorf("cleanup auction: %v", err)
		}
	})

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string
	if err := f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID); err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	if _, err := service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	); err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	authorization, err := postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("create authorization: %v", err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	providerID := "pi_flex_concurrent_" + uuid.NewString()

	authorized, err := postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		providerID,
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if authorized == nil || authorized.Status != "AUTHORIZED" {
		t.Fatalf("expected AUTHORIZED payment, got %+v", authorized)
	}

	const attempts = 2

	type confirmationResult struct {
		tripID string
		err    error
	}

	start := make(chan struct{})
	results := make(chan confirmationResult, attempts)

	var wg sync.WaitGroup
	wg.Add(attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			<-start

			trip, err := service.ConfirmAuthorizedRide(
				ctx,
				f.RideID,
				f.AuctionID,
				passengerID,
			)

			result := confirmationResult{err: err}
			if trip != nil {
				result.tripID = trip.ID
			}
			results <- result
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	rejections := 0
	var successfulTripID string

	for result := range results {
		switch {
		case result.err == nil:
			if result.tripID == "" {
				t.Fatal("successful confirmation returned no trip ID")
			}
			successes++
			successfulTripID = result.tripID

		case errors.Is(result.err, ErrFlexConfirmationNotEligible):
			if result.tripID != "" {
				t.Fatalf(
					"rejected confirmation returned trip ID %s",
					result.tripID,
				)
			}
			rejections++

		default:
			t.Fatalf(
				"unexpected concurrent confirmation result: trip=%s err=%v",
				result.tripID,
				result.err,
			)
		}
	}

	if successes != 1 || rejections != 1 {
		t.Fatalf(
			"expected one success and one rejection; successes=%d rejections=%d",
			successes,
			rejections,
		)
	}

	var tripCount int
	var persistedTripID string
	var persistedTripStatus string

	if err := f.DB.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id::text), MAX(status)
		FROM trips
		WHERE ride_request_id = $1
	`, f.RideID).Scan(
		&tripCount,
		&persistedTripID,
		&persistedTripStatus,
	); err != nil {
		t.Fatalf("read concurrent confirmation trip: %v", err)
	}

	if tripCount != 1 ||
		persistedTripID != successfulTripID ||
		persistedTripStatus != "ASSIGNED" {
		t.Fatalf(
			"unexpected trip state: count=%d id=%s status=%s winner=%s",
			tripCount,
			persistedTripID,
			persistedTripStatus,
			successfulTripID,
		)
	}

	var rideStatus string
	var offerStatus string
	var auctionStatus string
	var presenceStatus string
	var authorizationStatus string

	checks := []struct {
		name  string
		query string
		id    string
		dest  *string
	}{
		{
			"ride",
			"SELECT status FROM ride_requests WHERE id = $1",
			f.RideID,
			&rideStatus,
		},
		{
			"offer",
			"SELECT status FROM flex_offers WHERE id = $1",
			offer.ID,
			&offerStatus,
		},
		{
			"auction",
			"SELECT status FROM flex_auctions WHERE id = $1",
			f.AuctionID,
			&auctionStatus,
		},
		{
			"presence",
			"SELECT availability_status FROM driver_presence WHERE driver_id = $1",
			f.DriverID,
			&presenceStatus,
		},
		{
			"authorization",
			"SELECT status FROM flex_authorizations WHERE id = $1",
			authorization.ID,
			&authorizationStatus,
		},
	}

	for _, check := range checks {
		if err := f.DB.QueryRow(
			ctx, check.query, check.id,
		).Scan(check.dest); err != nil {
			t.Fatalf("read %s after concurrent confirmation: %v", check.name, err)
		}
	}

	if rideStatus != "ACCEPTED" ||
		offerStatus != "ACCEPTED" ||
		auctionStatus != "CONFIRMED" ||
		presenceStatus != "BUSY" ||
		authorizationStatus != "AUTHORIZED" {
		t.Fatalf(
			"unexpected concurrent confirmation state: ride=%s offer=%s auction=%s presence=%s authorization=%s",
			rideStatus,
			offerStatus,
			auctionStatus,
			presenceStatus,
			authorizationStatus,
		)
	}
}
