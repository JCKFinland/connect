package trip

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestUpdateStatusCompletedReleasesBusyDriver(t *testing.T) {
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
	// 3. Serialize access to John's shared integration fixture.
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
	// 4. Controlled existing fixture IDs.
	//
	// trips.driver_id and driver_presence.driver_id use users.id.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	// ---------------------------------------------------------
	// 5. Ensure John has DRIVER authorization for the real
	//    UpdateStatus service path.
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
	// 6. Avoid interfering with an existing active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	if err := db.QueryRow(
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
	); err != nil {
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
	// 7. Preserve John's current presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
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
	); err != nil {
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
	// 8. Prepare BUSY driver state.
	// ---------------------------------------------------------

	if _, err := db.Exec(
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
	); err != nil {
		t.Fatalf(
			"prepare BUSY driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Create disposable ACCEPTED ride request.
	//
	// Completing the trip must leave this ride ACCEPTED.
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
				'Trip Completion Release Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'Terminal trip must release BUSY driver',
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
			"create completion test ride request: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Create disposable IN_PROGRESS active trip.
	//
	// IN_PROGRESS -> COMPLETED is a valid lifecycle transition.
	// ---------------------------------------------------------

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
		johnVehicleID,
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

	// ---------------------------------------------------------
	// 11. Cleanup order:
	//
	// trip_events -> trips -> ride_requests.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trip_events
				WHERE trip_id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup completion trip events: %v",
				cleanupErr,
			)
		}

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
	// 12. Construct the real trip service.
	// ---------------------------------------------------------

	tripRepo :=
		postgresrepo.NewTripRepository(db)

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	tripEventRepo :=
		postgresrepo.NewTripEventRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Trips:        tripRepo,
			RideRequests: rideRequestRepo,
			Presence:     presenceRepo,
			TripEvents:   tripEventRepo,
			UserRoles:    userRoleRepo,
		},
	)

	// ---------------------------------------------------------
	// 13. Complete the trip through the actual service.
	// ---------------------------------------------------------

	err = service.UpdateStatus(
		ctx,
		tripID,
		StatusCompleted,
		johnUserID,
	)
	if err != nil {
		t.Fatalf(
			"complete trip: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 14. Verify terminal trip state.
	// ---------------------------------------------------------

	var (
		tripStatus  string
		isActive    bool
		completedAt *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				status,
				is_active,
				completed_at
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&tripStatus,
		&isActive,
		&completedAt,
	); err != nil {
		t.Fatalf(
			"read completed trip: %v",
			err,
		)
	}

	if tripStatus != StatusCompleted {
		t.Fatalf(
			"expected trip status %s, got %s",
			StatusCompleted,
			tripStatus,
		)
	}

	if isActive {
		t.Fatal(
			"expected completed trip to be inactive",
		)
	}

	if completedAt == nil {
		t.Fatal(
			"expected completed_at to be populated",
		)
	}

	// ---------------------------------------------------------
	// 15. Verify BUSY driver was released to AVAILABLE.
	// ---------------------------------------------------------

	var (
		isOnline           bool
		availabilityStatus string
	)

	if err := db.QueryRow(
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
	); err != nil {
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

	if availabilityStatus != driverAvailabilityAvailable {
		t.Fatalf(
			"expected driver status %s, got %s",
			driverAvailabilityAvailable,
			availabilityStatus,
		)
	}

	// ---------------------------------------------------------
	// 16. Completing a trip does not rewrite ride_requests.
	// ---------------------------------------------------------

	var rideStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&rideStatus,
	); err != nil {
		t.Fatalf(
			"read completed trip ride status: %v",
			err,
		)
	}

	if rideStatus != "ACCEPTED" {
		t.Fatalf(
			"expected source ride to remain ACCEPTED, got %s",
			rideStatus,
		)
	}

	// ---------------------------------------------------------
	// 17. Verify immutable lifecycle event exists.
	// ---------------------------------------------------------

	var eventCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_events
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&eventCount,
	); err != nil {
		t.Fatalf(
			"count completion trip events: %v",
			err,
		)
	}

	if eventCount != 1 {
		t.Fatalf(
			"expected exactly 1 completion lifecycle event, got %d",
			eventCount,
		)
	}
}
