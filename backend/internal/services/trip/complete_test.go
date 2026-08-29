package trip

import (
	"context"
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
	// 3. Serialize John's shared integration fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)
	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	// ---------------------------------------------------------
	// 4. Controlled fixture IDs.
	//
	// trips.driver_id and driver_presence.driver_id use users.id.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	// ---------------------------------------------------------
	// 5. John must have DRIVER authorization.
	// ---------------------------------------------------------

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	roles, err := userRoleRepo.GetUserRoles(
		ctx,
		johnUserID,
	)
	if err != nil {
		t.Fatalf(
			"load John roles: %v",
			err,
		)
	}

	hasDriverRole := false

	for _, role := range roles {
		if role == "DRIVER" {
			hasDriverRole = true
			break
		}
	}

	if !hasDriverRole {
		t.Skip(
			"John fixture does not currently have DRIVER role",
		)
	}

	// ---------------------------------------------------------
	// 6. Resolve John's current active assignment.
	//
	// Do not hardcode vehicle/fleet ownership because other
	// integration tests may legitimately evolve the fixture.
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
			  AND status = 'ACTIVE'
			  AND is_active = TRUE
			  AND released_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		`,
		johnUserID,
	).Scan(
		&vehicleID,
		&companyID,
		&branchID,
		&fleetID,
	)
	if err != nil {
		t.Fatalf(
			"resolve John's active assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Avoid interfering with a genuine active trip.
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
		johnUserID,
	).Scan(
		&existingActiveTripCount,
	)
	if err != nil {
		t.Fatalf(
			"check existing active trip: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Skip(
			"John already has an active trip",
		)
	}

	// ---------------------------------------------------------
	// 8. Preserve John's presence.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	)
	if err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Put John into the BUSY state expected during a trip.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'BUSY',
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	)
	if err != nil {
		t.Fatalf(
			"prepare BUSY driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Create disposable ride + IN_PROGRESS trip.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				1,
				'ACCEPTED',
				'Fare failure must roll back completion',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
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
				'IN_PROGRESS',
				111,
				222,
				$9,
				$9,
				TRUE,
				$9,
				$9
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		johnUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create rollback test trip: %v",
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
				1.00,
				1.00,
				'EUR',
				1.00,
				'rollback-fixture',
				$2
			)
		`,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create conflicting trip fare: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 12. Cleanup.
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
	}()

	// ---------------------------------------------------------
	// 13. Construct real production service dependencies.
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
	// 14. Attempt completion.
	//
	// Metrics will be updated inside the transaction first.
	// Fare persistence must then fail on UNIQUE(trip_id).
	// ---------------------------------------------------------

	completedFare, err := service.CompleteTrip(
		ctx,
		tripID,
		johnUserID,
		CompleteTripInput{
			ActualDistanceMeters: 8420,

			ActualDurationSeconds: 900,

			WaitingDurationSeconds: 150,

			BaseFare: 4.90,

			DistanceRatePerKM: 1.50,

			TimeRatePerMinute: 0.25,

			WaitingRatePerMinute: 0.50,

			BookingFee: 2.00,

			SurgeMultiplier: 1.00,

			TaxAmount: 5.17,

			Currency: "EUR",

			PricingVersion: "v1",
		},
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
	// 15. Trip lifecycle and metrics must have rolled back.
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
	// 16. No completion event may survive.
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
	// 17. Driver must remain BUSY.
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
		johnUserID,
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
	// 18. Only the pre-existing fare must remain.
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
	// 3. Serialize John's shared integration fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)
	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	// ---------------------------------------------------------
	// 4. Verify DRIVER authorization.
	// ---------------------------------------------------------

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	roles, err := userRoleRepo.GetUserRoles(
		ctx,
		johnUserID,
	)
	if err != nil {
		t.Fatalf(
			"load John roles: %v",
			err,
		)
	}

	hasDriverRole := false

	for _, role := range roles {
		if role == "DRIVER" {
			hasDriverRole = true
			break
		}
	}

	if !hasDriverRole {
		t.Skip(
			"John fixture does not currently have DRIVER role",
		)
	}

	// ---------------------------------------------------------
	// 5. Resolve current active assignment.
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
			  AND status = 'ACTIVE'
			  AND is_active = TRUE
			  AND released_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		`,
		johnUserID,
	).Scan(
		&vehicleID,
		&companyID,
		&branchID,
		&fleetID,
	)
	if err != nil {
		t.Fatalf(
			"resolve John's active assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 6. Do not interfere with a genuine active trip.
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
		johnUserID,
	).Scan(
		&existingActiveTripCount,
	)
	if err != nil {
		t.Fatalf(
			"check existing active trip: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Skip(
			"John already has an active trip",
		)
	}

	// ---------------------------------------------------------
	// 7. Preserve presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	)
	if err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 8. Driver is BUSY while carrying the passenger.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'BUSY',
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	)
	if err != nil {
		t.Fatalf(
			"prepare BUSY driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Create disposable ride and IN_PROGRESS trip.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()
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
				1,
				'ACCEPTED',
				'Successful atomic fare completion test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
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
				'IN_PROGRESS',
				$9,
				$9,
				TRUE,
				$9,
				$9
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		johnUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create completion test trip: %v",
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
		johnUserID,
		CompleteTripInput{
			ActualDistanceMeters: 8420,

			ActualDurationSeconds: 900,

			WaitingDurationSeconds: 150,

			BaseFare: 4.90,

			DistanceRatePerKM: 1.50,

			TimeRatePerMinute: 0.25,

			WaitingRatePerMinute: 0.50,

			BookingFee: 2.00,

			SurgeMultiplier: 1.00,

			TaxAmount: 5.17,

			Currency: "EUR",

			PricingVersion: "v1",
		},
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

	if completedFare.TotalAmount != 29.70 {
		t.Fatalf(
			"expected fare total 29.70, got %.2f",
			completedFare.TotalAmount,
		)
	}

	// ---------------------------------------------------------
	// 12. Verify authoritative completed trip measurements.
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

	if actualDistanceMeters == nil ||
		*actualDistanceMeters != 8420 {
		t.Fatalf(
			"expected actual distance 8420, got %v",
			actualDistanceMeters,
		)
	}

	if actualDurationSeconds == nil ||
		*actualDurationSeconds != 900 {
		t.Fatalf(
			"expected actual duration 900, got %v",
			actualDurationSeconds,
		)
	}

	// ---------------------------------------------------------
	// 13. Verify persisted fare snapshot.
	// ---------------------------------------------------------

	var (
		fareCount              int
		totalAmount            float64
		chargedDistanceMeters  int64
		chargedDurationSeconds int64
		waitingSeconds         int64
		pricingVersion         string
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
				MAX(pricing_version)
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
		&pricingVersion,
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

	if totalAmount != 29.70 {
		t.Fatalf(
			"expected persisted total 29.70, got %.2f",
			totalAmount,
		)
	}

	if chargedDistanceMeters != 8420 {
		t.Fatalf(
			"expected charged distance 8420, got %d",
			chargedDistanceMeters,
		)
	}

	if chargedDurationSeconds != 900 {
		t.Fatalf(
			"expected charged duration 900, got %d",
			chargedDurationSeconds,
		)
	}

	if waitingSeconds != 150 {
		t.Fatalf(
			"expected waiting duration 150, got %d",
			waitingSeconds,
		)
	}

	if pricingVersion != "v1" {
		t.Fatalf(
			"expected pricing version v1, got %s",
			pricingVersion,
		)
	}

	// ---------------------------------------------------------
	// 14. Exactly one completion event must exist.
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

	// ---------------------------------------------------------
	// 15. Driver must be released to AVAILABLE.
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
		johnUserID,
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
	// 16. Completion does not rewrite ride request status.
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
