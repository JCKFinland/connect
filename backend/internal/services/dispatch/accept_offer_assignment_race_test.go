package dispatch

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"errors"
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestAcceptOfferSerializesAssignmentRemoval(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatalf(
			"heartbeat timeout must be greater than zero, got %v",
			cfg.Presence.HeartbeatTimeout,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create isolated driver fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupDriverFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup isolated driver fixture: %v",
				err,
			)
		}
	}()

	complianceCleanup, err :=
		testutil.MakeDriverRegulatorilyCompliant(
			ctx,
			db,
			driverFixture,
		)
	if err != nil {
		t.Fatalf(
			"make isolated driver regulatorily compliant: %v",
			err,
		)
	}

	defer func() {
		if err := complianceCleanup(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup isolated driver compliance fixture: %v",
				err,
			)
		}
	}()

	driverUserID := driverFixture.UserID

	freshHeartbeat := time.Now().UTC()

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO driver_presence (
			driver_id,
			company_id,
			branch_id,
			vehicle_id,
			assignment_id,
			is_online,
			availability_status,
			latitude,
			longitude,
			last_heartbeat_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			TRUE,
			'AVAILABLE',
			60.2055,
			24.6559,
			$6
		)
	`,
		driverUserID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
		driverFixture.VehicleID,
		driverFixture.AssignmentID,
		freshHeartbeat,
	)
	if err != nil {
		t.Fatalf(
			"create isolated fresh-heartbeat driver presence: %v",
			err,
		)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`
			DELETE FROM driver_presence
			WHERE driver_id = $1
		`,
			driverUserID,
		); err != nil {
			t.Logf(
				"cleanup isolated driver presence: %v",
				err,
			)
		}
	}()

	// ---------------------------------------------------------
	// Create disposable PENDING STANDARD ride.
	//
	// Pickup coordinates exactly match John's coordinates.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()

	pricingProfileID := uuid.NewString()
	pricingVersion := "test-" + uuid.NewString()

	// ---------------------------------------------------------
	// Create disposable service category for propagation test.
	// ---------------------------------------------------------

	companyID := driverFixture.CompanyID

	serviceCategoryID := uuid.NewString()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO service_categories
			(
				id,
				code,
				name,
				description,
				is_active,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'DispatchRide Test Category',
				'DispatchRide service-category propagation regression test',
				TRUE,
				NOW(),
				NOW()
			)
		`,
		serviceCategoryID,
		"DISPATCH_TEST_"+uuid.NewString()[:8],
	)

	if err != nil {
		t.Fatalf(
			"create DispatchRide service category: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO fare_pricing_profiles
			(
				id,
				company_id,
				branch_id,
				service_category_id,
				version,
				currency,
				base_fare,
				distance_rate_per_km,
				time_rate_per_minute,
				waiting_rate_per_minute,
				booking_fee,
				surge_multiplier,
				effective_from,
				effective_to,
				is_active,
				created_at
			)
			VALUES
			(
				$1,
				$2,
				NULL,
				$3,
				$4,
				'EUR',
				5.00,
				1.50,
				0.50,
				0.50,
				0.00,
				1.00,
				$5,
				NULL,
				TRUE,
				$5
			)
		`,
		pricingProfileID,
		companyID,
		serviceCategoryID,
		pricingVersion,
		time.Now().UTC().Add(-time.Hour),
	)

	if err != nil {
		t.Fatalf(
			"create DispatchRide fresh-heartbeat pricing profile: %v",
			err,
		)
	}

	now := time.Now().UTC()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO ride_requests
			(
				id,
				customer_id,
				pickup_address,
				pickup_latitude,
				pickup_longitude,
				destination_address,
				destination_latitude,
				destination_longitude,
				requested_vehicle_type,
                service_category_id,
                passenger_count,
				status,
				notes,
				requested_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'DispatchRide Compliance Recheck Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
                $3,
                1,
                'MATCHING',
				'DispatchRide post-ranking compliance regression test',
				$4,
				$5,
				$4,
				$4
			)
		`,
		rideRequestID,
		customerID,
		serviceCategoryID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create DispatchRide fresh-heartbeat ride: %v",
			err,
		)
	}

	// Clean up in FK-safe order:
	// trip -> ride request -> pricing profile -> service category.

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(cleanupCtx,
			`DELETE FROM trips WHERE ride_request_id = $1`, rideRequestID); cleanupErr != nil {
			t.Logf("cleanup accepted trip: %v", cleanupErr)
		}

		// Remove created offer before the referenced ride request.
		if _, cleanupErr := db.Exec(cleanupCtx,
			`DELETE FROM dispatch_offers WHERE ride_request_id = $1`,
			rideRequestID); cleanupErr != nil {
			t.Logf("cleanup created dispatch offer: %v", cleanupErr)
		}

		// 2. Ride request references the service category.
		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide fresh-heartbeat ride: %v",
				cleanupErr,
			)
		}

		// 3. Pricing profile references the service category.
		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM fare_pricing_profiles
				WHERE id = $1
			`,
			pricingProfileID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide pricing profile: %v",
				cleanupErr,
			)
		}

		// 4. Nothing created by this test should reference
		//    the service category now.
		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM service_categories
				WHERE id = $1
			`,
			serviceCategoryID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide service category: %v",
				cleanupErr,
			)
		}
	}()

	offerID := uuid.NewString()
	_, err = db.Exec(ctx, `INSERT INTO dispatch_offers
		(id, ride_request_id, driver_id, vehicle_id, company_id, branch_id, fleet_id,
		 status, offered_at, expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'PENDING',$8,$9,$8,$8)`,
		offerID, rideRequestID, driverFixture.DriverID, driverFixture.VehicleID,
		driverFixture.CompanyID, driverFixture.BranchID, driverFixture.FleetID,
		now, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("create pending offer: %v", err)
	}

	service := NewService(
		Dependencies{
			DB:     db,
			Config: cfg,
		},
	)

	// Resolve the verified taxi-driver license belonging to this fixture.
	assignmentService := assignment.NewService(assignment.Dependencies{DB: db})

	type dispatchResult struct {
		trip *models.Trip
		err  error
	}

	type unassignResult struct{ err error }

	selected := make(chan struct{})
	releaseDispatch := make(chan struct{})
	dispatchDone := make(chan dispatchResult, 1)
	unassignDone := make(chan unassignResult, 1)

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseDispatch) })
	}
	defer release()

	service.afterSelectCandidate = func(driverID string) {
		if driverID != driverUserID {
			return
		}
		close(selected)
		<-releaseDispatch
	}

	go func() {
		offer, err := service.AcceptOffer(ctx, offerID)
		dispatchDone <- dispatchResult{trip: offer, err: err}
	}()

	select {
	case <-selected:
	case result := <-dispatchDone:
		t.Fatalf("dispatch ended before selecting isolated driver: %v", result.err)
	case <-time.After(15 * time.Second):
		t.Fatal("dispatch did not select isolated compliant driver")
	}

	unassignStarted := make(chan struct{})
	go func() {
		close(unassignStarted)
		err := assignmentService.Unassign(context.Background(), driverFixture.UserID)
		unassignDone <- unassignResult{err: err}
	}()
	<-unassignStarted
	select {
	case result := <-unassignDone:
		t.Fatalf("assignment removal completed while acceptance held driver lock: %v", result.err)
	case <-time.After(150 * time.Millisecond):
		// Unassignment must wait for the acceptance transaction.
	}
	var assignmentActive bool
	if err := db.QueryRow(ctx, `SELECT unassigned_at IS NULL FROM driver_assignments WHERE id = $1`, driverFixture.AssignmentID).Scan(&assignmentActive); err != nil {
		t.Fatalf("read assignment while acceptance holds lock: %v", err)
	}
	if !assignmentActive {
		t.Fatalf("assignment changed before acceptance committed")
	}

	release()

	var dispatched dispatchResult
	select {
	case dispatched = <-dispatchDone:
	case <-time.After(15 * time.Second):
		t.Fatal("dispatch did not complete after releasing synchronization point")
	}

	if dispatched.err != nil {
		t.Fatalf("dispatch failed: %v", dispatched.err)
	}
	if dispatched.trip == nil {
		t.Fatal("offer acceptance returned no trip")
	}
	if dispatched.trip.DriverID != driverFixture.UserID {
		t.Fatalf(
			"expected isolated driver %s, got %s",
			driverFixture.UserID,
			dispatched.trip.DriverID,
		)
	}

	select {
	case result := <-unassignDone:
		if !errors.Is(result.err, presence.ErrDriverAvailabilityLocked) {
			t.Fatalf("expected busy-driver assignment protection after acceptance, got %v", result.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("unassignment did not complete after acceptance committed")
	}
	var finalActive bool
	if err := db.QueryRow(ctx, `SELECT unassigned_at IS NULL FROM driver_assignments WHERE id = $1`, driverFixture.AssignmentID).Scan(&finalActive); err != nil {
		t.Fatalf("read final assignment status: %v", err)
	}
	if !finalActive {
		t.Fatalf("accepted trip lost active assignment")
	}
}
