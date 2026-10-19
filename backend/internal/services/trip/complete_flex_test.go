package trip

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

type completedFlexLifecycle struct {
	Fixture         *testutil.FlexAcceptFixture
	TripID          string
	AuthorizationID string
	ProviderID      string
	AmountCents     int64
	Currency        string
}

type preparedFlexLifecycle struct {
	Fixture          *testutil.FlexAcceptFixture
	TripService      Service
	TripID           string
	AuthorizationID  string
	ProviderID       string
	AmountCents      int64
	Currency         string
	PricingVersion   string
	PricingProfileID string
}

func TestFlexTripAuthorizedLifecycle(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	completeAuthorizedFlexLifecycle(t)
}

func prepareAuthorizedFlexLifecycle(t *testing.T) *preparedFlexLifecycle {
	t.Helper()

	f := testutil.SetupFlexAcceptFixture(t)

	testutil.RegisterFlexTripCleanup(
		t,
		f.DB,
		f.RideID,
		f.AuctionID,
	)

	ctx := context.Background()

	flexService := flex_offer.NewService(
		flex_offer.Dependencies{
			DB:     f.DB,
			Config: f.Config,
		},
	)

	offer, err := flexService.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
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

	_, err = flexService.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	authorization, err :=
		postgresrepo.CreateFlexAuthorization(
			ctx,
			f.DB,
			f.RideID,
			f.AuctionID,
			passengerID,
		)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx,
			f.DB,
			authorization.ID,
		)

	if err != nil || !claimed {
		t.Fatalf(
			"claim FLEX authorization: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	authorized, err :=
		postgresrepo.ReconcileFlexAuthorization(
			ctx,
			f.DB,
			authorization.ID,
			"pi_flex_trip_"+uuid.NewString(),
			postgresrepo.FlexResultAuthorized,
		)

	if err != nil {
		t.Fatalf("authorize FLEX payment: %v", err)
	}

	if authorized == nil || authorized.Status != "AUTHORIZED" {
		t.Fatalf(
			"unexpected authorization: %+v",
			authorized,
		)
	}

	confirmedTrip, err :=
		flexService.ConfirmAuthorizedRide(
			ctx,
			f.RideID,
			f.AuctionID,
			passengerID,
		)

	if err != nil {
		t.Fatalf("confirm authorized FLEX ride: %v", err)
	}

	if confirmedTrip == nil ||
		confirmedTrip.Status != StatusAssigned ||
		confirmedTrip.DriverID != f.DriverID {
		t.Fatalf(
			"unexpected confirmed FLEX trip: %+v",
			confirmedTrip,
		)
	}

	tripService := NewService(
		Dependencies{
			DB: f.DB,

			Trips: postgresrepo.NewTripRepository(f.DB),

			RideRequests: postgresrepo.NewRideRequestRepository(f.DB),

			Presence: postgresrepo.NewDriverPresenceRepository(f.DB),

			TripEvents: postgresrepo.NewTripEventRepository(f.DB),

			UserRoles: repository.NewUserRoleRepository(f.DB),

			TripLocations: postgresrepo.NewTripLocationRepository(f.DB),

			FareCalculator: fare.NewService(),
		},
	)

	for _, status := range []string{
		StatusDriverEnRoute,
		StatusDriverArrived,
		StatusInProgress,
	} {
		if err := tripService.UpdateStatus(
			ctx,
			confirmedTrip.ID,
			status,
			f.DriverID,
		); err != nil {
			t.Fatalf(
				"transition FLEX trip to %s: %v",
				status,
				err,
			)
		}
	}

	currentTrip, err := tripService.GetByID(
		ctx,
		confirmedTrip.ID,
	)
	if err != nil {
		t.Fatalf("read FLEX trip: %v", err)
	}

	if currentTrip.Status != StatusInProgress {
		t.Fatalf(
			"expected IN_PROGRESS, got %s",
			currentTrip.Status,
		)
	}

	contract, err :=
		postgresrepo.ResolveConfirmedFlexFareContract(
			ctx,
			f.DB,
			confirmedTrip.ID,
		)

	if err != nil {
		t.Fatalf("resolve authorized FLEX contract: %v", err)
	}

	if contract.AmountCents != offer.OfferedAmountCents {
		t.Fatalf(
			"authorized fare mismatch: expected %d cents, got %d",
			offer.OfferedAmountCents,
			contract.AmountCents,
		)
	}

	// FLEX GPS AND COMPLETION ASSERTIONS
	//
	// Record actual journey evidence through the production GPS API.
	// The agreed FLEX fare must not be recalculated from this evidence.

	accuracy := 5.0
	speed := 25.0

	recordedAt := time.Now().UTC().
		Add(-20 * time.Second).
		Truncate(time.Microsecond)

	samples := []RecordLocationRequest{
		{
			Latitude:       60.1708,
			Longitude:      24.9375,
			AccuracyMeters: &accuracy,
			SpeedKMH:       &speed,
			RecordedAt:     recordedAt,
		},
		{
			Latitude:       60.1710,
			Longitude:      24.9380,
			AccuracyMeters: &accuracy,
			SpeedKMH:       &speed,
			RecordedAt:     recordedAt.Add(10 * time.Second),
		},
	}

	for index, sample := range samples {
		location, err := tripService.RecordTripLocation(
			ctx,
			confirmedTrip.ID,
			f.DriverID,
			sample,
		)
		if err != nil {
			t.Fatalf(
				"record FLEX GPS sample %d: %v",
				index+1,
				err,
			)
		}
		if location == nil || location.ID == "" {
			t.Fatalf(
				"missing persisted FLEX GPS sample %d",
				index+1,
			)
		}
	}

	if authorized.ProviderAuthorizationID == nil ||
		*authorized.ProviderAuthorizationID == "" {
		t.Fatal("authorized FLEX payment has no provider identity")
	}

	return &preparedFlexLifecycle{
		Fixture:          f,
		TripService:      tripService,
		TripID:           confirmedTrip.ID,
		AuthorizationID:  authorized.ID,
		ProviderID:       *authorized.ProviderAuthorizationID,
		AmountCents:      contract.AmountCents,
		Currency:         contract.Currency,
		PricingVersion:   contract.PricingVersion,
		PricingProfileID: contract.PricingProfileID,
	}
}

func completeAuthorizedFlexLifecycle(t *testing.T) *completedFlexLifecycle {
	t.Helper()

	prepared := prepareAuthorizedFlexLifecycle(t)
	ctx := context.Background()

	completedFare, err := prepared.TripService.CompleteTrip(
		ctx,
		prepared.TripID,
		prepared.Fixture.DriverID,
	)
	if err != nil {
		t.Fatalf("complete authorized FLEX trip: %v", err)
	}
	if completedFare == nil {
		t.Fatal("completed FLEX trip returned no fare")
	}

	expectedAmount := float64(prepared.AmountCents) / 100

	if completedFare.TotalAmount != expectedAmount {
		t.Fatalf(
			"FLEX fare changed: expected %.2f, got %.2f",
			expectedAmount,
			completedFare.TotalAmount,
		)
	}

	if completedFare.Currency != prepared.Currency {
		t.Fatalf(
			"FLEX currency mismatch: expected %s, got %s",
			prepared.Currency,
			completedFare.Currency,
		)
	}

	if completedFare.PricingVersion != prepared.PricingVersion {
		t.Fatalf(
			"FLEX pricing version mismatch: expected %s, got %s",
			prepared.PricingVersion,
			completedFare.PricingVersion,
		)
	}

	if completedFare.PricingProfileID == nil ||
		*completedFare.PricingProfileID != prepared.PricingProfileID {
		t.Fatalf(
			"FLEX pricing profile mismatch: %+v",
			completedFare.PricingProfileID,
		)
	}

	var storedFareAmount float64
	var storedPricingVersion string
	var storedPricingProfileID string

	err = prepared.Fixture.DB.QueryRow(ctx, `
		SELECT total_amount,
		       pricing_version,
		       pricing_profile_id
		FROM trip_fares
		WHERE trip_id = $1
	`, prepared.TripID).Scan(
		&storedFareAmount,
		&storedPricingVersion,
		&storedPricingProfileID,
	)
	if err != nil {
		t.Fatalf("read persisted FLEX fare: %v", err)
	}

	if storedFareAmount != expectedAmount ||
		storedPricingVersion != prepared.PricingVersion ||
		storedPricingProfileID != prepared.PricingProfileID {
		t.Fatalf(
			"persisted FLEX fare mismatch: amount=%.2f version=%s profile=%s",
			storedFareAmount,
			storedPricingVersion,
			storedPricingProfileID,
		)
	}

	var distanceMeters int64
	var durationSeconds int64
	var acceptedSamples int

	err = prepared.Fixture.DB.QueryRow(ctx, `
		SELECT distance_meters,
		       duration_seconds,
		       accepted_samples
		FROM trip_meter_measurements
		WHERE trip_id = $1
	`, prepared.TripID).Scan(
		&distanceMeters,
		&durationSeconds,
		&acceptedSamples,
	)
	if err != nil {
		t.Fatalf("read persisted FLEX meter evidence: %v", err)
	}

	if distanceMeters <= 0 ||
		durationSeconds <= 0 ||
		acceptedSamples < 2 {
		t.Fatalf(
			"invalid FLEX meter evidence: distance=%d duration=%d accepted=%d",
			distanceMeters,
			durationSeconds,
			acceptedSamples,
		)
	}

	completedTrip, err := prepared.TripService.GetByID(
		ctx,
		prepared.TripID,
	)
	if err != nil {
		t.Fatalf("read completed FLEX trip: %v", err)
	}

	if completedTrip.Status != StatusCompleted {
		t.Fatalf(
			"expected COMPLETED, got %s",
			completedTrip.Status,
		)
	}

	var availabilityStatus string

	err = prepared.Fixture.DB.QueryRow(ctx, `
		SELECT availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, prepared.Fixture.DriverID).Scan(&availabilityStatus)
	if err != nil {
		t.Fatalf("read released FLEX driver: %v", err)
	}

	if availabilityStatus == "BUSY" {
		t.Fatalf(
			"FLEX driver remained BUSY after completion",
		)
	}

	return &completedFlexLifecycle{
		Fixture:         prepared.Fixture,
		TripID:          prepared.TripID,
		AuthorizationID: prepared.AuthorizationID,
		ProviderID:      prepared.ProviderID,
		AmountCents:     prepared.AmountCents,
		Currency:        prepared.Currency,
	}

}

// TestFlexTripRejectsInvalidAuctionContract verifies that a FLEX
// auction invalidated after trip confirmation cannot produce
// a completed fare, meter snapshot, or driver release.
//
// The test modifies only the isolated FLEX fixture's authorization.
func TestFlexTripRejectsInvalidAuctionContract(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)

	testutil.RegisterFlexTripCleanup(
		t,
		f.DB,
		f.RideID,
		f.AuctionID,
	)

	ctx := context.Background()

	flexService := flex_offer.NewService(
		flex_offer.Dependencies{
			DB:     f.DB,
			Config: f.Config,
		},
	)

	offer, err := flexService.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}
	if offer == nil {
		t.Fatal("FLEX offer is nil")
	}

	var passengerID string
	if err := f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID); err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	if _, err := flexService.SelectPassengerOffer(
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
			"claim FLEX authorization: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	authorized, err := postgresrepo.ReconcileFlexAuthorization(
		ctx,
		f.DB,
		authorization.ID,
		"pi_flex_rollback_"+uuid.NewString(),
		postgresrepo.FlexResultAuthorized,
	)
	if err != nil {
		t.Fatalf("authorize FLEX payment: %v", err)
	}
	if authorized == nil || authorized.Status != "AUTHORIZED" {
		t.Fatalf("unexpected authorization: %+v", authorized)
	}

	confirmedTrip, err := flexService.ConfirmAuthorizedRide(
		ctx,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("confirm authorized FLEX ride: %v", err)
	}
	if confirmedTrip == nil || confirmedTrip.Status != StatusAssigned {
		t.Fatalf("unexpected confirmed trip: %+v", confirmedTrip)
	}

	tripService := NewService(Dependencies{
		DB:             f.DB,
		Trips:          postgresrepo.NewTripRepository(f.DB),
		RideRequests:   postgresrepo.NewRideRequestRepository(f.DB),
		Presence:       postgresrepo.NewDriverPresenceRepository(f.DB),
		TripEvents:     postgresrepo.NewTripEventRepository(f.DB),
		UserRoles:      repository.NewUserRoleRepository(f.DB),
		TripLocations:  postgresrepo.NewTripLocationRepository(f.DB),
		FareCalculator: fare.NewService(),
	})

	for _, status := range []string{
		StatusDriverEnRoute,
		StatusDriverArrived,
		StatusInProgress,
	} {
		if err := tripService.UpdateStatus(
			ctx,
			confirmedTrip.ID,
			status,
			f.DriverID,
		); err != nil {
			t.Fatalf("transition FLEX trip to %s: %v", status, err)
		}
	}

	accuracy := 5.0
	speed := 25.0
	recordedAt := time.Now().UTC().
		Add(-20 * time.Second).
		Truncate(time.Microsecond)

	for index, sample := range []RecordLocationRequest{
		{
			Latitude:       60.1708,
			Longitude:      24.9375,
			AccuracyMeters: &accuracy,
			SpeedKMH:       &speed,
			RecordedAt:     recordedAt,
		},
		{
			Latitude:       60.1710,
			Longitude:      24.9380,
			AccuracyMeters: &accuracy,
			SpeedKMH:       &speed,
			RecordedAt:     recordedAt.Add(10 * time.Second),
		},
	} {
		if _, err := tripService.RecordTripLocation(
			ctx,
			confirmedTrip.ID,
			f.DriverID,
			sample,
		); err != nil {
			t.Fatalf("record FLEX GPS sample %d: %v", index+1, err)
		}
	}

	// Confirm the contract was valid before invalidation.
	if _, err := postgresrepo.ResolveConfirmedFlexFareContract(
		ctx,
		f.DB,
		confirmedTrip.ID,
	); err != nil {
		t.Fatalf("expected valid initial FLEX contract: %v", err)
	}

	// Simulate invalidation of the confirmed FLEX auction contract.
	// The payment authorization remains immutable and AUTHORIZED.
	// Only the isolated fixture's auction is modified.
	tag, err := f.DB.Exec(ctx, `
		UPDATE flex_auctions
		SET status = 'CANCELLED',
		    updated_at = clock_timestamp()
		WHERE id = $1
		  AND status = 'CONFIRMED'
	`, f.AuctionID)
	if err != nil {
		t.Fatalf("invalidate fixture FLEX auction: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf(
			"expected one invalidated FLEX auction, changed %d",
			tag.RowsAffected(),
		)
	}

	// Verify the original authorization was not changed.
	var authorizationStatus string
	if err := f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&authorizationStatus); err != nil {
		t.Fatalf("read immutable FLEX authorization: %v", err)
	}
	if authorizationStatus != "AUTHORIZED" {
		t.Fatalf(
			"authorization unexpectedly changed: %s",
			authorizationStatus,
		)
	}

	completedFare, err := tripService.CompleteTrip(
		ctx,
		confirmedTrip.ID,
		f.DriverID,
	)
	if err == nil {
		t.Fatal("expected FLEX completion to reject invalid auction contract")
	}
	if completedFare != nil {
		t.Fatalf("unexpected completed fare: %+v", completedFare)
	}

	// The completion transaction must have rolled back its meter
	// snapshot, metrics, fare, lifecycle event, and driver release.
	var status string
	var active bool
	var actualDistance, actualDuration *int64

	if err := f.DB.QueryRow(ctx, `
		SELECT status,
		       is_active,
		       actual_distance_meters,
		       actual_duration_seconds
		FROM trips
		WHERE id = $1
	`, confirmedTrip.ID).Scan(
		&status,
		&active,
		&actualDistance,
		&actualDuration,
	); err != nil {
		t.Fatalf("read rejected FLEX trip: %v", err)
	}

	if status != StatusInProgress || !active {
		t.Fatalf(
			"trip mutated after failed completion: status=%s active=%v",
			status,
			active,
		)
	}
	if actualDistance != nil || actualDuration != nil {
		t.Fatalf(
			"completion metrics persisted despite rollback: distance=%v duration=%v",
			actualDistance,
			actualDuration,
		)
	}

	for _, check := range []struct {
		name  string
		query string
	}{
		{
			name:  "trip fare",
			query: "SELECT count(*) FROM trip_fares WHERE trip_id = $1",
		},
		{
			name:  "meter snapshot",
			query: "SELECT count(*) FROM trip_meter_measurements WHERE trip_id = $1",
		},
		{
			name: "completion event",
			query: `
				SELECT count(*)
				FROM trip_events
				WHERE trip_id = $1
				  AND event_type = 'TRIP_COMPLETED'
			`,
		},
	} {
		var count int
		if err := f.DB.QueryRow(
			ctx,
			check.query,
			confirmedTrip.ID,
		).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != 0 {
			t.Fatalf(
				"%s persisted after failed completion: %d records",
				check.name,
				count,
			)
		}
	}

	var availabilityStatus string
	if err := f.DB.QueryRow(ctx, `
		SELECT availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, f.DriverID).Scan(&availabilityStatus); err != nil {
		t.Fatalf("read driver presence after rollback: %v", err)
	}
	if availabilityStatus != "BUSY" {
		t.Fatalf(
			"driver released despite failed completion: %s",
			availabilityStatus,
		)
	}
}
