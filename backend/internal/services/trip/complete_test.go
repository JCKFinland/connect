package trip

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestCompleteTripRollsBackWhenFarePersistenceFails(
	t *testing.T,
) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	// ---------------------------------------------------------
	// 3. Create an isolated driver fixture.
	// ---------------------------------------------------------

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

	driverUserID := driverFixture.UserID

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	// ---------------------------------------------------------
	// 5. Ensure the isolated user has the DRIVER role.
	// ---------------------------------------------------------

	var driverRoleID string

	err = db.QueryRow(
		ctx,
		`
				SELECT id
				FROM roles
				WHERE name = 'DRIVER'
				LIMIT 1
			`,
	).Scan(
		&driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"resolve DRIVER role: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
				INSERT INTO user_roles
				(
					user_id,
					role_id
				)
				VALUES
				(
					$1,
					$2
				)
				ON CONFLICT DO NOTHING
			`,
		driverUserID,
		driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"grant DRIVER role to isolated user: %v",
			err,
		)
	}

	var (
		vehicleID string
		companyID string
		branchID  string
		fleetID   string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				vehicle_id,
				company_id,
				branch_id,
				fleet_id
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		`,
		driverUserID,
	).Scan(
		&vehicleID,
		&companyID,
		&branchID,
		&fleetID,
	)
	if err != nil {
		t.Fatalf(
			"resolve isolated driver's active assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 6. Assert the isolated driver starts without an active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				'COMPLETED',
				'CANCELLED',
				'NO_DRIVER_AVAILABLE',
				'EXPIRED'
			  )
		`,
		driverUserID,
	).Scan(
		&existingActiveTripCount,
	)
	if err != nil {
		t.Fatalf(
			"check isolated driver's active trips: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Fatalf(
			"isolated driver unexpectedly has %d active trip(s)",
			existingActiveTripCount,
		)
	}

	// ---------------------------------------------------------
	// 7. Create BUSY online presence for the isolated driver.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence
			(
				driver_id,
				company_id,
				branch_id,
				vehicle_id,
				assignment_id,
				is_online,
				availability_status,
				last_heartbeat_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1, $2, $3, $4, $5,
				TRUE, 'BUSY', NOW(), NOW(), NOW()
			)
		`,
		driverUserID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
		driverFixture.VehicleID,
		driverFixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf(
			"create isolated BUSY driver presence: %v",
			err,
		)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			driverUserID,
		); err != nil {
			t.Logf(
				"cleanup isolated driver presence: %v",
				err,
			)
		}
	}()
	// ---------------------------------------------------------
	// 8. Create disposable pricing authority, ride, and trip.
	// ---------------------------------------------------------

	serviceCategoryID := uuid.NewString()
	pricingProfileID := uuid.NewString()
	pricingVersion := "rollback-" + uuid.NewString()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

	now := time.Now().UTC()
	pricingEffectiveFrom := now.Add(-time.Hour)

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
				'Fare Rollback Test Category',
				'Frozen pricing rollback integration test',
				TRUE,
				$3,
				$3
			)
		`,
		serviceCategoryID,
		"ROLLBACK_"+uuid.NewString()[:8],
		now,
	)
	if err != nil {
		t.Fatalf(
			"create rollback service category: %v",
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
				$3,
				$4,
				$5,
				'EUR',
				4.90,
				1.50,
				0.25,
				0.50,
				2.00,
				1.00,
				$6,
				NULL,
				TRUE,
				$7
			)
		`,
		pricingProfileID,
		companyID,
		branchID,
		serviceCategoryID,
		pricingVersion,
		pricingEffectiveFrom,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create rollback pricing profile: %v",
			err,
		)
	}

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
				'Fare Rollback Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				$3,
				1,
				'ACCEPTED',
				'Fare failure must roll back completion',
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
			"create rollback ride request: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips
			(
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				service_category_id,
				pricing_profile_id,
				status,
				actual_distance_meters,
				actual_duration_seconds,
				assigned_at,
				started_at,
				is_active,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9,
				$10,
				'IN_PROGRESS',
				111,
				222,
				$11,
				$11,
				TRUE,
				$11,
				$11
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		serviceCategoryID,
		pricingProfileID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create rollback test trip: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. Create authoritative trip location evidence.
	//
	// These samples are intentionally simple and valid so
	// CompleteTrip reaches fare persistence, where this test
	// expects the UNIQUE(trip_id) failure.
	// ---------------------------------------------------------

	location1Time := now.Add(1 * time.Minute)
	location2Time := now.Add(2 * time.Minute)

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO trip_locations (
			id,
			trip_id,
			driver_id,
			latitude,
			longitude,
			accuracy_meters,
			recorded_at
		)
		VALUES
			($1, $2, $3, $4, $5, $6, $7),
			($8, $2, $3, $9, $10, $6, $11)
	`,
		uuid.NewString(),
		tripID,
		driverUserID,
		60.1700,
		24.9300,
		5.0,
		location1Time,
		uuid.NewString(),
		60.1710,
		24.9310,
		location2Time,
	)
	if err != nil {
		t.Fatalf(
			"create rollback trip location evidence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. Pre-create the authoritative fare.
	//
	// CompleteTrip will later attempt another INSERT for this
	// trip and hit UNIQUE(trip_id).
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trip_fares
			(
				trip_id,
				pricing_profile_id,
				base_fare,
				total_amount,
				currency,
				surge_multiplier,
				pricing_version,
				calculated_at
			)
			VALUES
			(
				$1,
				$2,
				1.00,
				1.00,
				'EUR',
				1.00,
				'rollback-fixture',
				$3
			)
		`,
		tripID,
		pricingProfileID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create conflicting trip fare: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 13. Cleanup.
	//
	// Deleting the trip cascades trip_fares and trip_events.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup rollback trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup rollback ride request: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM fare_pricing_profiles
				WHERE id = $1
			`,
			pricingProfileID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup rollback pricing profile: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM service_categories
				WHERE id = $1
			`,
			serviceCategoryID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup rollback service category: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 14. Construct real production service dependencies.
	// ---------------------------------------------------------

	service := NewService(
		Dependencies{
			DB: db,

			Trips: postgresrepo.NewTripRepository(db),

			RideRequests: postgresrepo.NewRideRequestRepository(db),

			Presence: postgresrepo.NewDriverPresenceRepository(db),

			TripEvents: postgresrepo.NewTripEventRepository(db),

			UserRoles: userRoleRepo,

			FareCalculator: fare.NewService(),
		},
	)
	// ---------------------------------------------------------
	// 15. Attempt completion.
	//
	// Metrics will be updated inside the transaction first.
	// Fare persistence must then fail on UNIQUE(trip_id).
	// ---------------------------------------------------------

	completedFare, err := service.CompleteTrip(
		ctx,
		tripID,
		driverUserID,
	)

	if err == nil {
		t.Fatal(
			"expected trip completion to fail",
		)
	}

	if completedFare != nil {
		t.Fatal(
			"expected no completed fare after rollback",
		)
	}

	if !strings.Contains(
		err.Error(),
		"persist trip fare",
	) {
		t.Fatalf(
			"expected fare persistence failure, got: %v",
			err,
		)
	}
	// ---------------------------------------------------------
	// 16. Trip lifecycle and metrics must have rolled back.
	// ---------------------------------------------------------

	var (
		status                string
		isActive              bool
		completedAt           *time.Time
		actualDistanceMeters  *int64
		actualDurationSeconds *int64
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				is_active,
				completed_at,
				actual_distance_meters,
				actual_duration_seconds
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&status,
		&isActive,
		&completedAt,
		&actualDistanceMeters,
		&actualDurationSeconds,
	)
	if err != nil {
		t.Fatalf(
			"read trip after rollback: %v",
			err,
		)
	}

	if status != StatusInProgress {
		t.Fatalf(
			"expected trip to remain %s, got %s",
			StatusInProgress,
			status,
		)
	}

	if !isActive {
		t.Fatal(
			"expected trip to remain active",
		)
	}

	if completedAt != nil {
		t.Fatal(
			"expected completed_at to remain NULL",
		)
	}

	if actualDistanceMeters == nil ||
		*actualDistanceMeters != 111 {
		t.Fatalf(
			"expected distance to roll back to 111, got %v",
			actualDistanceMeters,
		)
	}

	if actualDurationSeconds == nil ||
		*actualDurationSeconds != 222 {
		t.Fatalf(
			"expected duration to roll back to 222, got %v",
			actualDurationSeconds,
		)
	}

	// ---------------------------------------------------------
	// 17. No completion event may survive.
	// ---------------------------------------------------------

	var completionEventCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_events
			WHERE trip_id = $1
			  AND event_type = $2
		`,
		tripID,
		EventTripCompleted,
	).Scan(
		&completionEventCount,
	)
	if err != nil {
		t.Fatalf(
			"count completion events after rollback: %v",
			err,
		)
	}

	if completionEventCount != 0 {
		t.Fatalf(
			"expected no completion event after rollback, got %d",
			completionEventCount,
		)
	}

	// ---------------------------------------------------------
	// 18. Driver must remain BUSY.
	// ---------------------------------------------------------

	var (
		isOnline           bool
		availabilityStatus string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		driverUserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
	)
	if err != nil {
		t.Fatalf(
			"read driver presence after rollback: %v",
			err,
		)
	}

	if !isOnline {
		t.Fatal(
			"expected driver to remain online",
		)
	}

	if availabilityStatus != "BUSY" {
		t.Fatalf(
			"expected driver to remain BUSY, got %s",
			availabilityStatus,
		)
	}

	// ---------------------------------------------------------
	// 19. Meter audit snapshot must also roll back.
	//
	// CompleteTrip persists the authoritative meter snapshot
	// before fare persistence. Because fare persistence failed,
	// the transaction must leave no measurement snapshot behind.
	// ---------------------------------------------------------

	var meterMeasurementCount int

	err = db.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM trip_meter_measurements
		WHERE trip_id = $1
	`,
		tripID,
	).Scan(
		&meterMeasurementCount,
	)
	if err != nil {
		t.Fatalf(
			"count trip meter measurements after rollback: %v",
			err,
		)
	}

	if meterMeasurementCount != 0 {
		t.Fatalf(
			"expected no trip meter measurement after rollback, got %d",
			meterMeasurementCount,
		)
	}

	// ---------------------------------------------------------
	// 20. Only the pre-existing fare must remain.
	// ---------------------------------------------------------

	var fareCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_fares
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&fareCount,
	)
	if err != nil {
		t.Fatalf(
			"count fares after rollback: %v",
			err,
		)
	}

	if fareCount != 1 {
		t.Fatalf(
			"expected exactly the pre-existing fare, got %d",
			fareCount,
		)
	}
}

func TestCompleteTripFinalizesFareAndReleasesDriver(
	t *testing.T,
) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()
	// ---------------------------------------------------------
	// 3. Create an isolated driver fixture.
	// ---------------------------------------------------------

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

	driverUserID := driverFixture.UserID

	// ---------------------------------------------------------
	// 4. Ensure the isolated user has DRIVER authorization.
	// ---------------------------------------------------------

	var driverRoleID string

	err = db.QueryRow(
		ctx,
		`
			SELECT id
			FROM roles
			WHERE name = 'DRIVER'
			LIMIT 1
		`,
	).Scan(
		&driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"resolve DRIVER role: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO user_roles
			(
				user_id,
				role_id
			)
			VALUES
			(
				$1,
				$2
			)
			ON CONFLICT DO NOTHING
		`,
		driverUserID,
		driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"grant DRIVER role to isolated user: %v",
			err,
		)
	}

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	// ---------------------------------------------------------
	// 5. Resolve the isolated driver's active assignment.
	// ---------------------------------------------------------

	var (
		vehicleID string
		companyID string
		branchID  string
		fleetID   string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				vehicle_id,
				company_id,
				branch_id,
				fleet_id
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		`,
		driverUserID,
	).Scan(
		&vehicleID,
		&companyID,
		&branchID,
		&fleetID,
	)
	if err != nil {
		t.Fatalf(
			"resolve isolated driver's active assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 6. Assert the isolated driver starts without an active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				'COMPLETED',
				'CANCELLED',
				'NO_DRIVER_AVAILABLE',
				'EXPIRED'
			  )
		`,
		driverUserID,
	).Scan(
		&existingActiveTripCount,
	)
	if err != nil {
		t.Fatalf(
			"check isolated driver's active trips: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Fatalf(
			"isolated driver unexpectedly has %d active trip(s)",
			existingActiveTripCount,
		)
	}

	// ---------------------------------------------------------
	// 7. Create BUSY online presence for the isolated driver.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence
			(
				driver_id,
				company_id,
				branch_id,
				vehicle_id,
				assignment_id,
				is_online,
				availability_status,
				last_heartbeat_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1, $2, $3, $4, $5,
				TRUE, 'BUSY', NOW(), NOW(), NOW()
			)
		`,
		driverUserID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
		driverFixture.VehicleID,
		driverFixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf(
			"create isolated BUSY driver presence: %v",
			err,
		)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			driverUserID,
		); err != nil {
			t.Logf(
				"cleanup isolated driver presence: %v",
				err,
			)
		}
	}()
	// ---------------------------------------------------------
	// 9. Create disposable pricing authority, ride, and trip.
	// ---------------------------------------------------------

	serviceCategoryID := uuid.NewString()
	pricingProfileID := uuid.NewString()
	pricingVersion := "completion-" + uuid.NewString()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()
	now := time.Now().UTC()
	pricingEffectiveFrom := now.Add(-time.Hour)

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
				'Fare Completion Test Category',
				'Frozen pricing completion integration test',
				TRUE,
				$3,
				$3
			)
		`,
		serviceCategoryID,
		"COMPLETE_"+uuid.NewString()[:8],
		now,
	)
	if err != nil {
		t.Fatalf(
			"create completion service category: %v",
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
				$3,
				$4,
				$5,
				'EUR',
				4.90,
				1.50,
				0.25,
				0.50,
				2.00,
				1.00,
				$6,
				NULL,
				TRUE,
				$7
			)
		`,
		pricingProfileID,
		companyID,
		branchID,
		serviceCategoryID,
		pricingVersion,
		pricingEffectiveFrom,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create completion pricing profile: %v",
			err,
		)
	}

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
				'Fare Completion Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				$3,
				1,
				'ACCEPTED',
				'Successful atomic fare completion test',
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
			"create completion ride request: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips
			(
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				service_category_id,
				pricing_profile_id,
				status,
				assigned_at,
				started_at,
				is_active,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9,
				$10,
				'IN_PROGRESS',
				$11,
				$11,
				TRUE,
				$11,
				$11
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		serviceCategoryID,
		pricingProfileID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create completion test trip: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Create authoritative trip location evidence.
	//
	// These persisted GPS samples deliberately produce
	// measurements are derived exclusively from persisted trip evidence.
	// This proves completion uses server-side trip evidence,
	// not caller-supplied distance/time/waiting values.
	// ---------------------------------------------------------

	location1Time := now.Add(1 * time.Minute)
	location2Time := now.Add(2 * time.Minute)
	location3Time := now.Add(3 * time.Minute)

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO trip_locations (
			id,
			trip_id,
			driver_id,
			latitude,
			longitude,
			accuracy_meters,
			recorded_at
		)
		VALUES
			($1,  $2, $3, $4, $5, $6, $7),
			($8,  $2, $3, $9, $10, $6, $11),
			($12, $2, $3, $13, $14, $6, $15)
	`,
		uuid.NewString(),
		tripID,
		driverUserID,
		60.170000,
		24.930000,
		5.0,
		location1Time,

		uuid.NewString(),
		60.170500,
		24.930000,
		location2Time,

		uuid.NewString(),
		60.170500,
		24.930000,
		location3Time,
	)
	if err != nil {
		t.Fatalf(
			"create completion trip location evidence: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup completion trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup completion ride request: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM fare_pricing_profiles
				WHERE id = $1
			`,
			pricingProfileID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup completion pricing profile: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM service_categories
				WHERE id = $1
			`,
			serviceCategoryID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup completion service category: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 10. Construct real service.
	// ---------------------------------------------------------

	service := NewService(
		Dependencies{
			DB: db,

			Trips: postgresrepo.NewTripRepository(db),

			RideRequests: postgresrepo.NewRideRequestRepository(db),

			Presence: postgresrepo.NewDriverPresenceRepository(db),

			TripEvents: postgresrepo.NewTripEventRepository(db),

			UserRoles: userRoleRepo,

			FareCalculator: fare.NewService(),
		},
	)
	// ---------------------------------------------------------
	// 11. Complete through the production transaction.
	// ---------------------------------------------------------

	completedFare, err := service.CompleteTrip(
		ctx,
		tripID,
		driverUserID,
	)
	if err != nil {
		t.Fatalf(
			"complete trip: %v",
			err,
		)
	}

	if completedFare == nil {
		t.Fatal(
			"expected completed fare",
		)
	}

	if completedFare.ID == "" {
		t.Fatal(
			"expected persisted fare ID",
		)
	}

	if completedFare.TripID != tripID {
		t.Fatalf(
			"expected fare trip ID %s, got %s",
			tripID,
			completedFare.TripID,
		)
	}

	if completedFare.PricingProfileID == nil {
		t.Fatal(
			"expected completed fare pricing profile id to be set",
		)
	}

	if *completedFare.PricingProfileID != pricingProfileID {
		t.Fatalf(
			"expected fare pricing profile id %s, got %s",
			pricingProfileID,
			*completedFare.PricingProfileID,
		)
	}

	// ---------------------------------------------------------
	// 12. Verify immutable authoritative meter snapshot.
	//
	// Successful completion must persist exactly one meter
	// snapshot containing the measurement used for both the
	// trip operational metrics and the finalized fare.
	// ---------------------------------------------------------

	var (
		meterMeasurementID   string
		measurementSource    string
		algorithmVersion     string
		meterDistanceMeters  int64
		meterDurationSeconds int64
		meterWaitingSeconds  int64
		acceptedSamples      int
		rejectedSamples      int
		rejectedSegments     int
		meterMeasuredAt      time.Time
		meterCreatedAt       time.Time
	)

	err = db.QueryRow(
		ctx,
		`
		SELECT
			id,
			measurement_source,
			algorithm_version,
			distance_meters,
			duration_seconds,
			waiting_duration_seconds,
			accepted_samples,
			rejected_samples,
			rejected_segments,
			measured_at,
			created_at
		FROM trip_meter_measurements
		WHERE trip_id = $1
	`,
		tripID,
	).Scan(
		&meterMeasurementID,
		&measurementSource,
		&algorithmVersion,
		&meterDistanceMeters,
		&meterDurationSeconds,
		&meterWaitingSeconds,
		&acceptedSamples,
		&rejectedSamples,
		&rejectedSegments,
		&meterMeasuredAt,
		&meterCreatedAt,
	)
	if err != nil {
		t.Fatalf(
			"read trip meter measurement after completion: %v",
			err,
		)
	}

	if meterMeasurementID == "" {
		t.Fatal(
			"expected persisted trip meter measurement ID",
		)
	}

	if measurementSource != "GPS" {
		t.Fatalf(
			"expected measurement source GPS, got %s",
			measurementSource,
		)
	}

	if algorithmVersion != "gps-v1" {
		t.Fatalf(
			"expected algorithm version gps-v1, got %s",
			algorithmVersion,
		)
	}

	if meterDistanceMeters != 56 {
		t.Fatalf(
			"expected meter distance 56, got %d",
			meterDistanceMeters,
		)
	}

	if meterDurationSeconds != 120 {
		t.Fatalf(
			"expected meter duration 120, got %d",
			meterDurationSeconds,
		)
	}

	if meterWaitingSeconds != 60 {
		t.Fatalf(
			"expected meter waiting duration 60, got %d",
			meterWaitingSeconds,
		)
	}

	if acceptedSamples != 3 {
		t.Fatalf(
			"expected 3 accepted samples, got %d",
			acceptedSamples,
		)
	}

	if rejectedSamples != 0 {
		t.Fatalf(
			"expected 0 rejected samples, got %d",
			rejectedSamples,
		)
	}

	if rejectedSegments != 0 {
		t.Fatalf(
			"expected 0 rejected segments, got %d",
			rejectedSegments,
		)
	}

	if meterMeasuredAt.IsZero() {
		t.Fatal(
			"expected meter measured_at to be set",
		)
	}

	if meterCreatedAt.IsZero() {
		t.Fatal(
			"expected meter created_at to be set",
		)
	}

	// ---------------------------------------------------------
	// 13. Verify authoritative completed trip measurements.
	// ---------------------------------------------------------

	var (
		status                string
		isActive              bool
		completedAt           *time.Time
		actualDistanceMeters  *int64
		actualDurationSeconds *int64
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				is_active,
				completed_at,
				actual_distance_meters,
				actual_duration_seconds
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&status,
		&isActive,
		&completedAt,
		&actualDistanceMeters,
		&actualDurationSeconds,
	)
	if err != nil {
		t.Fatalf(
			"read completed trip: %v",
			err,
		)
	}

	if status != StatusCompleted {
		t.Fatalf(
			"expected %s, got %s",
			StatusCompleted,
			status,
		)
	}

	if isActive {
		t.Fatal(
			"expected completed trip to be inactive",
		)
	}

	if completedAt == nil {
		t.Fatal(
			"expected completed_at",
		)
	}

	if actualDistanceMeters == nil {
		t.Fatal(
			"expected actual distance to be persisted",
		)
	}

	if *actualDistanceMeters != 56 {
		t.Fatalf(
			"expected authoritative actual distance 56, got %d",
			*actualDistanceMeters,
		)
	}

	if actualDurationSeconds == nil {
		t.Fatal(
			"expected actual duration to be persisted",
		)
	}

	if *actualDurationSeconds != 120 {
		t.Fatalf(
			"expected authoritative actual duration 120, got %d",
			*actualDurationSeconds,
		)
	}

	// ---------------------------------------------------------
	// 14. Verify persisted fare snapshot.
	// ---------------------------------------------------------

	var (
		fareCount                 int
		totalAmount               float64
		chargedDistanceMeters     int64
		chargedDurationSeconds    int64
		waitingSeconds            int64
		persistedPricingVersion   string
		persistedPricingProfileID *string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				COUNT(*),
				MAX(total_amount),
				MAX(charged_distance_meters),
				MAX(charged_duration_seconds),
				MAX(waiting_duration_seconds),
				MAX(pricing_version),
				MAX(pricing_profile_id::text)
			FROM trip_fares
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&fareCount,
		&totalAmount,
		&chargedDistanceMeters,
		&chargedDurationSeconds,
		&waitingSeconds,
		&persistedPricingVersion,
		&persistedPricingProfileID,
	)
	if err != nil {
		t.Fatalf(
			"read persisted fare: %v",
			err,
		)
	}

	if fareCount != 1 {
		t.Fatalf(
			"expected exactly 1 fare, got %d",
			fareCount,
		)
	}

	if totalAmount != 7.98 {
		t.Fatalf(
			"expected persisted total 7.98, got %.2f",
			totalAmount,
		)
	}

	if chargedDistanceMeters != 56 {
		t.Fatalf(
			"expected charged distance 56, got %d",
			chargedDistanceMeters,
		)
	}

	if chargedDurationSeconds != 120 {
		t.Fatalf(
			"expected charged duration 120, got %d",
			chargedDurationSeconds,
		)
	}

	if waitingSeconds != 60 {
		t.Fatalf(
			"expected waiting duration 60, got %d",
			waitingSeconds,
		)
	}

	if persistedPricingVersion != pricingVersion {
		t.Fatalf(
			"expected pricing version %s, got %s",
			pricingVersion,
			persistedPricingVersion,
		)
	}

	if persistedPricingProfileID == nil {
		t.Fatal(
			"expected persisted pricing profile id",
		)
	}

	if *persistedPricingProfileID != pricingProfileID {
		t.Fatalf(
			"expected persisted pricing profile id %s, got %s",
			pricingProfileID,
			*persistedPricingProfileID,
		)
	}

	// ---------------------------------------------------------
	// 15. Exactly one completion event must exist.
	// ---------------------------------------------------------

	var completionEventCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_events
			WHERE trip_id = $1
			  AND event_type = $2
		`,
		tripID,
		EventTripCompleted,
	).Scan(
		&completionEventCount,
	)
	if err != nil {
		t.Fatalf(
			"count completion events: %v",
			err,
		)
	}

	if completionEventCount != 1 {
		t.Fatalf(
			"expected exactly 1 completion event, got %d",
			completionEventCount,
		)
	}

	var eventMetadata []byte

	err = db.QueryRow(
		ctx,
		`
		SELECT metadata
		FROM trip_events
		WHERE trip_id = $1
		  AND event_type = $2
		LIMIT 1
	`,
		tripID,
		EventTripCompleted,
	).Scan(
		&eventMetadata,
	)
	if err != nil {
		t.Fatalf(
			"read completion event metadata: %v",
			err,
		)
	}

	var completionMetadata map[string]string

	if err := json.Unmarshal(
		eventMetadata,
		&completionMetadata,
	); err != nil {
		t.Fatalf(
			"decode completion event metadata: %v",
			err,
		)
	}

	if completionMetadata["meter_measurement_id"] != meterMeasurementID {
		t.Fatalf(
			"expected completion event meter measurement id %s, got %s",
			meterMeasurementID,
			completionMetadata["meter_measurement_id"],
		)
	}

	// ---------------------------------------------------------
	// 16. Driver must be released to AVAILABLE.
	// ---------------------------------------------------------

	var (
		isOnline           bool
		availabilityStatus string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		driverUserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
	)
	if err != nil {
		t.Fatalf(
			"read released driver presence: %v",
			err,
		)
	}

	if !isOnline {
		t.Fatal(
			"expected released driver to remain online",
		)
	}

	if availabilityStatus !=
		driverAvailabilityAvailable {
		t.Fatalf(
			"expected driver %s, got %s",
			driverAvailabilityAvailable,
			availabilityStatus,
		)
	}

	// ---------------------------------------------------------
	// 17. Completion does not rewrite ride request status.
	// ---------------------------------------------------------

	var rideStatus string

	err = db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&rideStatus,
	)
	if err != nil {
		t.Fatalf(
			"read source ride status: %v",
			err,
		)
	}

	if rideStatus != "ACCEPTED" {
		t.Fatalf(
			"expected ride request to remain ACCEPTED, got %s",
			rideStatus,
		)
	}
}
