package presence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestGoOnlineRejectsBusyDriverWithActiveTrip(t *testing.T) {
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
	// 3. Serialize access to John's shared fixture.
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
	// 4. Existing controlled fixture IDs.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	// ---------------------------------------------------------
	// 5. Load John's active vehicle assignment.
	// ---------------------------------------------------------

	assignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	activeAssignment, err :=
		assignmentRepo.GetActiveByDriver(
			ctx,
			johnUserID,
		)

	if err != nil {
		t.Fatalf(
			"load John's active assignment: %v",
			err,
		)
	}

	if activeAssignment == nil ||
		activeAssignment.ID == "" ||
		activeAssignment.VehicleID == "" {

		t.Fatal(
			"John fixture requires an active vehicle assignment",
		)
	}

	// ---------------------------------------------------------
	// 6. Avoid interfering with a legitimate active trip.
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
	// 7. Preserve John's complete presence state.
	// ---------------------------------------------------------

	var (
		originalAssignmentID       *string
		originalVehicleID          *string
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalAssignmentID,
		&originalVehicleID,
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	if originalAssignmentID == nil ||
		*originalAssignmentID != activeAssignment.ID {

		t.Fatal(
			"John presence must reference his active assignment",
		)
	}

	if originalVehicleID == nil ||
		*originalVehicleID != activeAssignment.VehicleID {

		t.Fatal(
			"John presence must reference his assigned vehicle",
		)
	}

	// ---------------------------------------------------------
	// 8. Restore shared presence state when test finishes.
	// ---------------------------------------------------------

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					assignment_id = $2,
					vehicle_id = $3,
					is_online = $4,
					availability_status = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalAssignmentID,
			originalVehicleID,
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
	// 9. Put John into BUSY operational state.
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
	// 10. Create disposable ACCEPTED ride request.
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
				'Go Online Guard Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'GoOnline active-trip lifecycle guard test',
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
			"create GoOnline guard ride request: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. Create active ASSIGNED trip.
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
				'ASSIGNED',
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
		activeAssignment.VehicleID,
		activeAssignment.CompanyID,
		activeAssignment.BranchID,
		activeAssignment.FleetID,
		now,
	)

	if err != nil {
		t.Fatalf(
			"create active GoOnline guard trip: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 12. Remove disposable test data.
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
				"cleanup GoOnline guard trip: %v",
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
				"cleanup GoOnline guard ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 13. Construct the real presence service.
	//
	// GoOnline now requires DB because it runs transactionally.
	// ---------------------------------------------------------

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Presence:    presenceRepo,
			Assignments: assignmentRepo,
		},
	)

	// ---------------------------------------------------------
	// 14. GoOnline must NOT convert BUSY -> AVAILABLE.
	// ---------------------------------------------------------

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: johnUserID,
		},
	)

	if !errors.Is(
		err,
		ErrDriverAvailabilityLocked,
	) {
		t.Fatalf(
			"expected ErrDriverAvailabilityLocked, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 15. Presence must remain BUSY, online, and attached to the
	//     same assignment and vehicle.
	// ---------------------------------------------------------

	var (
		persistedAssignmentID       *string
		persistedVehicleID          *string
		persistedIsOnline           bool
		persistedAvailabilityStatus string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				assignment_id,
				vehicle_id,
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedAssignmentID,
		&persistedVehicleID,
		&persistedIsOnline,
		&persistedAvailabilityStatus,
	); err != nil {
		t.Fatalf(
			"read presence after rejected GoOnline: %v",
			err,
		)
	}

	if persistedAssignmentID == nil ||
		*persistedAssignmentID != activeAssignment.ID {

		t.Fatalf(
			"expected assignment_id %s to remain attached, got %v",
			activeAssignment.ID,
			persistedAssignmentID,
		)
	}

	if persistedVehicleID == nil ||
		*persistedVehicleID != activeAssignment.VehicleID {

		t.Fatalf(
			"expected vehicle_id %s to remain attached, got %v",
			activeAssignment.VehicleID,
			persistedVehicleID,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected BUSY driver to remain online",
		)
	}

	if persistedAvailabilityStatus != StatusBusy {
		t.Fatalf(
			"expected driver status %s, got %s",
			StatusBusy,
			persistedAvailabilityStatus,
		)
	}

	// ---------------------------------------------------------
	// 16. Assignment must remain active.
	// ---------------------------------------------------------

	var persistedUnassignedAt *time.Time

	if err := db.QueryRow(
		ctx,
		`
			SELECT unassigned_at
			FROM driver_assignments
			WHERE id = $1
		`,
		activeAssignment.ID,
	).Scan(
		&persistedUnassignedAt,
	); err != nil {
		t.Fatalf(
			"read assignment after rejected GoOnline: %v",
			err,
		)
	}

	if persistedUnassignedAt != nil {
		t.Fatalf(
			"expected assignment to remain active, got unassigned_at=%v",
			*persistedUnassignedAt,
		)
	}

	// ---------------------------------------------------------
	// 17. Active trip must remain untouched.
	// ---------------------------------------------------------

	var activeTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE id = $1
			  AND driver_id = $2
			  AND status = 'ASSIGNED'
			  AND is_active = TRUE
			  AND deleted_at IS NULL
		`,
		tripID,
		johnUserID,
	).Scan(
		&activeTripCount,
	); err != nil {
		t.Fatalf(
			"read trip after rejected GoOnline: %v",
			err,
		)
	}

	if activeTripCount != 1 {
		t.Fatalf(
			"expected active trip to remain intact, got %d",
			activeTripCount,
		)
	}
}
