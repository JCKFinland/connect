package dispatch

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/internal/services/fare"
	tripservice "github.com/JCKFinland/connect/backend/internal/services/trip"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestOperationalLifecycleAcceptedTripBlocksUnassign(t *testing.T) {
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
		(id, ride_request_id, driver_id, vehicle_id, company_id, branch_id, fleet_id, assignment_id,
		 status, offered_at, expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$10,'PENDING',$8,$9,$8,$8)`,
		offerID, rideRequestID, driverFixture.DriverID, driverFixture.VehicleID,
		driverFixture.CompanyID, driverFixture.BranchID, driverFixture.FleetID,
		now, now.Add(5*time.Minute), driverFixture.AssignmentID)
	if err != nil {
		t.Fatalf("create pending offer: %v", err)
	}

	assignmentService := assignment.NewService(assignment.Dependencies{DB: db})
	if err := assignmentService.Unassign(ctx, driverUserID); err != nil {
		t.Fatalf("unassign original assignment: %v", err)
	}
	replacement, err := assignmentService.Assign(ctx, driverUserID, assignment.AssignDriverRequest{VehicleID: driverFixture.VehicleID})
	if err != nil {
		t.Fatalf("reassign same vehicle: %v", err)
	}
	if replacement == nil || replacement.ID == "" || replacement.ID == driverFixture.AssignmentID {
		t.Fatalf("replacement assignment must have a new identity: %#v", replacement)
	}
	if replacement.CompanyID != driverFixture.CompanyID || replacement.BranchID != driverFixture.BranchID ||
		replacement.FleetID != driverFixture.FleetID || replacement.VehicleID != driverFixture.VehicleID {
		t.Fatal("replacement assignment changed vehicle or hierarchy")
	}
	defer func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM trips WHERE ride_request_id = $1`, rideRequestID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM dispatch_offers WHERE ride_request_id = $1`, rideRequestID)
		if _, err := db.Exec(cleanupCtx, `DELETE FROM driver_assignments WHERE id = $1`, replacement.ID); err != nil {
			t.Logf("cleanup replacement assignment: %v", err)
		}
	}()

	// Restore an available, fresh-heartbeat presence for the newly assigned
	// driver. Assignment transitions themselves are performed by the services.
	_, err = db.Exec(ctx, `UPDATE driver_presence SET is_online = TRUE,
        availability_status = 'AVAILABLE', last_heartbeat_at = NOW()
        WHERE driver_id = $1 AND assignment_id = $2`, driverUserID, replacement.ID)
	if err != nil {
		t.Fatalf("refresh replacement presence: %v", err)
	}

	var beforeAssignment string
	var beforeOnline bool
	var beforeAvailability string
	err = db.QueryRow(ctx, `SELECT assignment_id, is_online, availability_status
        FROM driver_presence WHERE driver_id = $1`, driverUserID).
		Scan(&beforeAssignment, &beforeOnline, &beforeAvailability)
	if err != nil {
		t.Fatalf("snapshot presence: %v", err)
	}
	if beforeAssignment != replacement.ID || !beforeOnline || beforeAvailability != "AVAILABLE" {
		t.Fatalf("replacement presence not ready: %s %v %s", beforeAssignment, beforeOnline, beforeAvailability)
	}

	service := NewService(Dependencies{DB: db, Config: cfg})
	trip, err := service.AcceptOffer(ctx, offerID)
	if !errors.Is(err, ErrDispatchOfferDriverNonCompliant) {
		t.Fatalf("expected stale assignment rejection, got trip=%v err=%v", trip, err)
	}
	if trip != nil {
		t.Fatalf("stale offer returned a trip: %v", trip)
	}

	var tripCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM trips WHERE ride_request_id = $1`, rideRequestID).Scan(&tripCount); err != nil {
		t.Fatalf("count trips after rejection: %v", err)
	}
	if tripCount != 0 {
		t.Fatalf("stale offer created %d trips", tripCount)
	}
	var afterAssignment, afterAvailability string
	var afterOnline bool
	if err := db.QueryRow(ctx, `SELECT assignment_id, is_online, availability_status
        FROM driver_presence WHERE driver_id = $1`, driverUserID).
		Scan(&afterAssignment, &afterOnline, &afterAvailability); err != nil {
		t.Fatalf("read presence after rejection: %v", err)
	}
	if afterAssignment != beforeAssignment || afterOnline != beforeOnline || afterAvailability != beforeAvailability {
		t.Fatalf("stale offer mutated presence: before=(%s,%v,%s), after=(%s,%v,%s)",
			beforeAssignment, beforeOnline, beforeAvailability, afterAssignment, afterOnline, afterAvailability)
	}

	// Retire the rejected offer so the same ride can receive a fresh offer.
	if _, err := db.Exec(ctx, `UPDATE dispatch_offers SET status = 'EXPIRED', updated_at = NOW()
        WHERE id = $1 AND status = 'PENDING'`, offerID); err != nil {
		t.Fatalf("retire stale offer: %v", err)
	}
	freshOfferID := uuid.NewString()
	_, err = db.Exec(ctx, `INSERT INTO dispatch_offers
        (id, ride_request_id, driver_id, vehicle_id, company_id, branch_id, fleet_id, assignment_id,
         status, offered_at, expires_at, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',NOW(),NOW() + INTERVAL '5 minutes',NOW(),NOW())`,
		freshOfferID, rideRequestID, driverFixture.DriverID, driverFixture.VehicleID,
		driverFixture.CompanyID, driverFixture.BranchID, driverFixture.FleetID, replacement.ID)
	if err != nil {
		t.Fatalf("create replacement-pinned offer: %v", err)
	}
	accepted, err := service.AcceptOffer(ctx, freshOfferID)
	if err != nil {
		t.Fatalf("accept replacement-pinned offer: %v", err)
	}
	if accepted == nil || accepted.DriverID != driverUserID {
		t.Fatalf("fresh offer did not create expected driver trip: %v", accepted)
	}

	var availability string
	var assignmentID string
	if err := db.QueryRow(ctx, `SELECT availability_status, assignment_id FROM driver_presence WHERE driver_id = $1`, driverUserID).Scan(&availability, &assignmentID); err != nil {
		t.Fatalf("read presence after acceptance: %v", err)
	}
	if availability != "BUSY" || assignmentID != replacement.ID {
		t.Fatalf("accepted trip must preserve assignment and set BUSY, got status=%s assignment=%s", availability, assignmentID)
	}
	if err := assignmentService.Unassign(ctx, driverUserID); err == nil {
		t.Fatal("active trip unexpectedly permitted driver unassignment")
	}
	var activeAssignmentID string
	if err := db.QueryRow(ctx, `SELECT id FROM driver_assignments WHERE driver_id = $1 AND unassigned_at IS NULL`, driverUserID).Scan(&activeAssignmentID); err != nil {
		t.Fatalf("read active assignment after rejected unassignment: %v", err)
	}
	if activeAssignmentID != replacement.ID {
		t.Fatalf("rejected unassignment changed active assignment: got %s want %s", activeAssignmentID, replacement.ID)
	}

	// Exercise the authoritative trip lifecycle rather than writing status
	// directly. The same isolated driver remains the actor throughout.
	var driverRoleID string
	if err := db.QueryRow(ctx, `SELECT id FROM roles WHERE name = 'DRIVER' LIMIT 1`).Scan(&driverRoleID); err != nil {
		t.Fatalf("resolve DRIVER role: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, driverUserID, driverRoleID); err != nil {
		t.Fatalf("grant isolated driver role: %v", err)
	}

	trips := tripservice.NewService(tripservice.Dependencies{
		DB:             db,
		Trips:          postgresrepo.NewTripRepository(db),
		RideRequests:   postgresrepo.NewRideRequestRepository(db),
		Presence:       postgresrepo.NewDriverPresenceRepository(db),
		TripEvents:     postgresrepo.NewTripEventRepository(db),
		UserRoles:      repository.NewUserRoleRepository(db),
		FareCalculator: fare.NewService(),
	})
	for _, state := range []string{
		tripservice.StatusDriverEnRoute,
		tripservice.StatusDriverArrived,
		tripservice.StatusInProgress,
	} {
		if err := trips.UpdateStatus(ctx, accepted.ID, state, driverUserID); err != nil {
			t.Fatalf("transition accepted trip to %s: %v", state, err)
		}
	}

	// Provide authoritative, persisted GPS evidence for fare calculation.
	// These are test-only measurements, not simulated status transitions.
	locationStart := time.Now().UTC().Add(-3 * time.Minute)
	if _, err := db.Exec(ctx, `INSERT INTO trip_locations
		(id, trip_id, driver_id, latitude, longitude, accuracy_meters, recorded_at)
		VALUES ($1,$2,$3,60.170000,24.930000,5.0,$4),
		       ($5,$2,$3,60.170500,24.930000,5.0,$6),
		       ($7,$2,$3,60.170500,24.930000,5.0,$8)`,
		uuid.NewString(), accepted.ID, driverUserID, locationStart,
		uuid.NewString(), locationStart.Add(time.Minute),
		uuid.NewString(), locationStart.Add(2*time.Minute)); err != nil {
		t.Fatalf("persist completion GPS evidence: %v", err)
	}

	completedFare, err := trips.CompleteTrip(ctx, accepted.ID, driverUserID)
	if err != nil {
		t.Fatalf("complete accepted trip through authoritative service: %v", err)
	}
	if completedFare == nil {
		t.Fatal("completion returned no finalized fare")
	}
	var finalStatus string
	var isActive bool
	if err := db.QueryRow(ctx, `SELECT status, is_active FROM trips WHERE id = $1`, accepted.ID).Scan(&finalStatus, &isActive); err != nil {
		t.Fatalf("read completed trip: %v", err)
	}
	if finalStatus != tripservice.StatusCompleted || isActive {
		t.Fatalf("trip not terminal: status=%s active=%v", finalStatus, isActive)
	}
	var releasedAssignment, releasedAvailability string
	var releasedOnline bool
	if err := db.QueryRow(ctx, `SELECT assignment_id, availability_status, is_online FROM driver_presence WHERE driver_id = $1`, driverUserID).Scan(&releasedAssignment, &releasedAvailability, &releasedOnline); err != nil {
		t.Fatalf("read released presence: %v", err)
	}
	if releasedAssignment != replacement.ID || releasedAvailability != "AVAILABLE" || !releasedOnline {
		t.Fatalf("completion failed to restore assigned AVAILABLE driver: assignment=%s availability=%s online=%v", releasedAssignment, releasedAvailability, releasedOnline)
	}
}
