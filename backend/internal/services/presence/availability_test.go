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

func TestGoOnlineReconcilesAssignmentAndBecomesAvailable(t *testing.T) {
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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	// ---------------------------------------------------------
	// 4. Load John's authoritative active assignment.
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
	// 5. GoOnline is only valid when no active trip exists.
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
	// 6. Preserve John's complete presence state.
	// ---------------------------------------------------------

	var (
		originalCompanyID          string
		originalBranchID           *string
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
				company_id,
				branch_id,
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
		&originalCompanyID,
		&originalBranchID,
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

	// ---------------------------------------------------------
	// 7. Restore John's presence after the test.
	// ---------------------------------------------------------

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					company_id = $2,
					branch_id = $3,
					assignment_id = $4,
					vehicle_id = $5,
					is_online = $6,
					availability_status = $7,
					last_heartbeat_at = $8,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalCompanyID,
			originalBranchID,
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
	// 8. Deliberately make presence stale.
	//
	// The active assignment remains authoritative.
	// GoOnline() must reconcile these fields.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				assignment_id = NULL,
				vehicle_id = NULL,
				is_online = FALSE,
				availability_status = 'OFFLINE',
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare stale OFFLINE presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Construct the real transactional presence service.
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
	// 10. Go online.
	// ---------------------------------------------------------

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: johnUserID,
		},
	)

	if err != nil {
		t.Fatalf(
			"go online with valid active assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. Verify assignment reconciliation and online state.
	// ---------------------------------------------------------

	var (
		persistedCompanyID          string
		persistedBranchID           *string
		persistedAssignmentID       *string
		persistedVehicleID          *string
		persistedIsOnline           bool
		persistedAvailabilityStatus string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedCompanyID,
		&persistedBranchID,
		&persistedAssignmentID,
		&persistedVehicleID,
		&persistedIsOnline,
		&persistedAvailabilityStatus,
	); err != nil {
		t.Fatalf(
			"read presence after GoOnline: %v",
			err,
		)
	}

	if persistedAssignmentID == nil ||
		*persistedAssignmentID != activeAssignment.ID {

		t.Fatalf(
			"expected assignment_id %s, got %v",
			activeAssignment.ID,
			persistedAssignmentID,
		)
	}

	if persistedVehicleID == nil ||
		*persistedVehicleID != activeAssignment.VehicleID {

		t.Fatalf(
			"expected vehicle_id %s, got %v",
			activeAssignment.VehicleID,
			persistedVehicleID,
		)
	}

	if persistedCompanyID != activeAssignment.CompanyID {
		t.Fatalf(
			"expected company_id %s, got %s",
			activeAssignment.CompanyID,
			persistedCompanyID,
		)
	}

	if persistedBranchID == nil ||
		*persistedBranchID != activeAssignment.BranchID {

		t.Fatalf(
			"expected branch_id %s, got %v",
			activeAssignment.BranchID,
			persistedBranchID,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected driver to be online",
		)
	}

	if persistedAvailabilityStatus != StatusAvailable {
		t.Fatalf(
			"expected driver status %s, got %s",
			StatusAvailable,
			persistedAvailabilityStatus,
		)
	}

	// ---------------------------------------------------------
	// 12. Active assignment itself must remain unchanged.
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
			"read assignment after successful GoOnline: %v",
			err,
		)
	}

	if persistedUnassignedAt != nil {
		t.Fatalf(
			"expected assignment to remain active, got unassigned_at=%v",
			*persistedUnassignedAt,
		)
	}
}

func TestUpdateAvailabilityRejectsDriverWithActiveTrip(t *testing.T) {
	ctx := context.Background()

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

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	// ---------------------------------------------------------
	// Preserve John's current presence state.
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
	// Avoid interfering with an existing active trip.
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
	// Set driver BUSY.
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
	// Create disposable active trip.
	// ---------------------------------------------------------

	tripID := uuid.NewString()
	now := time.Now().UTC()
	rideRequestID := uuid.NewString()

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
				'Presence Guard Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'Presence active-trip guard integration test',
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
			"create presence-guard ride request: %v",
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
		johnVehicleID,
		companyID,
		branchID,
		fleetID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create active presence-guard trip: %v",
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
				"cleanup active presence-guard trip: %v",
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
				"cleanup presence-guard ride request: %v",
				cleanupErr,
			)
		}
	}()

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			Config:   cfg,
			Presence: presenceRepo,
		},
	)

	// ---------------------------------------------------------
	// Manual AVAILABLE transition must be rejected.
	// ---------------------------------------------------------

	err = service.UpdateAvailability(
		ctx,
		UpdateAvailabilityRequest{
			UserID: johnUserID,
			Status: StatusAvailable,
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
	// Verify BUSY was preserved.
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
			"read guarded driver presence: %v",
			err,
		)
	}

	if !isOnline {
		t.Fatal(
			"expected driver to remain online while BUSY",
		)
	}

	if availabilityStatus != StatusBusy {
		t.Fatalf(
			"expected driver status %s, got %s",
			StatusBusy,
			availabilityStatus,
		)
	}
}

func TestGoOfflineRejectsDriverWithActiveTrip(t *testing.T) {
	ctx := context.Background()

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

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	// ---------------------------------------------------------
	// Preserve John's current presence state.
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
	// Avoid interfering with an existing active trip.
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
	// Set driver BUSY.
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
	// Create disposable active trip.
	// ---------------------------------------------------------

	tripID := uuid.NewString()
	now := time.Now().UTC()
	rideRequestID := uuid.NewString()

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
				'Presence Guard Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'Presence active-trip guard integration test',
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
			"create presence-guard ride request: %v",
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
		johnVehicleID,
		companyID,
		branchID,
		fleetID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create active presence-guard trip: %v",
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
				"cleanup active presence-guard trip: %v",
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
				"cleanup presence-guard ride request: %v",
				cleanupErr,
			)
		}
	}()

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			Config:   cfg,
			Presence: presenceRepo,
		},
	)

	// ---------------------------------------------------------
	// Going OFFLINE must also be rejected while the driver is
	// committed to an active trip.
	// ---------------------------------------------------------

	err = service.GoOffline(
		ctx,
		GoOfflineRequest{
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
	// Verify BUSY was preserved.
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
			"read guarded driver presence: %v",
			err,
		)
	}

	if !isOnline {
		t.Fatal(
			"expected driver to remain online while BUSY",
		)
	}

	if availabilityStatus != StatusBusy {
		t.Fatalf(
			"expected driver status %s, got %s",
			StatusBusy,
			availabilityStatus,
		)
	}
}

func TestGoOnlineRejectsDriverWithoutActiveAssignment(t *testing.T) {
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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	// ---------------------------------------------------------
	// 4. Load John's current active assignment.
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
		activeAssignment.ID == "" {

		t.Fatal(
			"John fixture requires an active assignment",
		)
	}

	// ---------------------------------------------------------
	// 5. Do not interfere with an existing active trip.
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
	// 6. Preserve John's presence state.
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

	// ---------------------------------------------------------
	// 7. Restore assignment + presence after the test.
	// ---------------------------------------------------------

	defer func() {
		restoreCtx := context.Background()

		if _, restoreErr := db.Exec(
			restoreCtx,
			`
				UPDATE driver_assignments
				SET
					unassigned_at = NULL,
					updated_at = NOW()
				WHERE id = $1
			`,
			activeAssignment.ID,
		); restoreErr != nil {
			t.Logf(
				"restore active assignment: %v",
				restoreErr,
			)
		}

		if _, restoreErr := db.Exec(
			restoreCtx,
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
	// 8. Close John's assignment.
	//
	// Presence deliberately remains unchanged so we prove that
	// a stale presence assignment cannot authorize GoOnline().
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET
				unassigned_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
		`,
		activeAssignment.ID,
	); err != nil {
		t.Fatalf(
			"close assignment for GoOnline test: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Force presence OFFLINE while leaving stale assignment
	//    and vehicle references intact.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = FALSE,
				availability_status = 'OFFLINE',
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare OFFLINE presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Construct real transactional presence service.
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
	// 11. GoOnline must reject the closed assignment.
	// ---------------------------------------------------------

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: johnUserID,
		},
	)

	if !errors.Is(
		err,
		ErrDriverAssignmentRequired,
	) {
		t.Fatalf(
			"expected ErrDriverAssignmentRequired, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 12. Presence must remain OFFLINE and unchanged.
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

	if persistedIsOnline {
		t.Fatal(
			"expected driver to remain offline",
		)
	}

	if persistedAvailabilityStatus != StatusOffline {
		t.Fatalf(
			"expected driver status %s, got %s",
			StatusOffline,
			persistedAvailabilityStatus,
		)
	}

	if originalAssignmentID == nil {
		if persistedAssignmentID != nil {
			t.Fatalf(
				"expected assignment_id to remain nil, got %v",
				persistedAssignmentID,
			)
		}
	} else {
		if persistedAssignmentID == nil ||
			*persistedAssignmentID != *originalAssignmentID {

			t.Fatalf(
				"expected stale assignment_id %s to remain unchanged, got %v",
				*originalAssignmentID,
				persistedAssignmentID,
			)
		}
	}

	if originalVehicleID == nil {
		if persistedVehicleID != nil {
			t.Fatalf(
				"expected vehicle_id to remain nil, got %v",
				persistedVehicleID,
			)
		}
	} else {
		if persistedVehicleID == nil ||
			*persistedVehicleID != *originalVehicleID {

			t.Fatalf(
				"expected vehicle_id %s to remain unchanged, got %v",
				*originalVehicleID,
				persistedVehicleID,
			)
		}
	}

	// ---------------------------------------------------------
	// 13. Confirm assignment remains closed during the test.
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
			"read closed assignment after rejected GoOnline: %v",
			err,
		)
	}

	if persistedUnassignedAt == nil {
		t.Fatal(
			"expected assignment to remain closed",
		)
	}
}

func TestGoOnlineRejectsMissingPresenceRow(t *testing.T) {
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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	// ---------------------------------------------------------
	// 4. Confirm John still has an active assignment.
	//
	// This proves the failure is specifically caused by missing
	// presence, not by missing assignment.
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
		activeAssignment.ID == "" {

		t.Fatal(
			"John fixture requires an active assignment",
		)
	}

	// ---------------------------------------------------------
	// 5. Avoid interfering with a legitimate active trip.
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
	// 6. Preserve the COMPLETE driver_presence row.
	//
	// We save every column because this test temporarily deletes
	// the shared fixture row and must restore it exactly.
	// ---------------------------------------------------------

	var (
		originalDriverID           string
		originalCompanyID          string
		originalBranchID           *string
		originalVehicleID          *string
		originalAssignmentID       *string
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeading            *float64
		originalSpeed              *float64
		originalAccuracy           *float64
		originalHeartbeat          *time.Time
		originalCreatedAt          time.Time
		originalUpdatedAt          time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				driver_id,
				company_id,
				branch_id,
				vehicle_id,
				assignment_id,
				is_online,
				availability_status,
				latitude,
				longitude,
				heading,
				speed,
				accuracy,
				last_heartbeat_at,
				created_at,
				updated_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalDriverID,
		&originalCompanyID,
		&originalBranchID,
		&originalVehicleID,
		&originalAssignmentID,
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeading,
		&originalSpeed,
		&originalAccuracy,
		&originalHeartbeat,
		&originalCreatedAt,
		&originalUpdatedAt,
	); err != nil {
		t.Fatalf(
			"load original driver presence row: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Always restore the deleted presence row.
	// ---------------------------------------------------------

	defer func() {
		restoreCtx := context.Background()

		if _, restoreErr := db.Exec(
			restoreCtx,
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
					latitude,
					longitude,
					heading,
					speed,
					accuracy,
					last_heartbeat_at,
					created_at,
					updated_at
				)
				VALUES
				(
					$1,$2,$3,$4,$5,
					$6,$7,$8,$9,$10,
					$11,$12,$13,$14,$15
				)
				ON CONFLICT (driver_id)
				DO UPDATE SET
					company_id = EXCLUDED.company_id,
					branch_id = EXCLUDED.branch_id,
					vehicle_id = EXCLUDED.vehicle_id,
					assignment_id = EXCLUDED.assignment_id,
					is_online = EXCLUDED.is_online,
					availability_status = EXCLUDED.availability_status,
					latitude = EXCLUDED.latitude,
					longitude = EXCLUDED.longitude,
					heading = EXCLUDED.heading,
					speed = EXCLUDED.speed,
					accuracy = EXCLUDED.accuracy,
					last_heartbeat_at = EXCLUDED.last_heartbeat_at,
					created_at = EXCLUDED.created_at,
					updated_at = EXCLUDED.updated_at
			`,
			originalDriverID,
			originalCompanyID,
			originalBranchID,
			originalVehicleID,
			originalAssignmentID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeading,
			originalSpeed,
			originalAccuracy,
			originalHeartbeat,
			originalCreatedAt,
			originalUpdatedAt,
		); restoreErr != nil {
			t.Logf(
				"restore deleted driver presence row: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 8. Temporarily delete John's presence row.
	// ---------------------------------------------------------

	result, err := db.Exec(
		ctx,
		`
			DELETE FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	)

	if err != nil {
		t.Fatalf(
			"delete driver presence for missing-row test: %v",
			err,
		)
	}

	if result.RowsAffected() != 1 {
		t.Fatalf(
			"expected one driver presence row to be deleted, got %d",
			result.RowsAffected(),
		)
	}

	// ---------------------------------------------------------
	// 9. Construct real transactional presence service.
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
	// 10. GoOnline must report missing driver presence.
	// ---------------------------------------------------------

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: johnUserID,
		},
	)

	if !errors.Is(
		err,
		ErrDriverNotFound,
	) {
		t.Fatalf(
			"expected ErrDriverNotFound, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. GoOnline must not recreate presence implicitly.
	// ---------------------------------------------------------

	var presenceCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&presenceCount,
	); err != nil {
		t.Fatalf(
			"count presence after rejected GoOnline: %v",
			err,
		)
	}

	if presenceCount != 0 {
		t.Fatalf(
			"expected presence row to remain absent during test, got %d",
			presenceCount,
		)
	}

	// ---------------------------------------------------------
	// 12. Active assignment must remain untouched.
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
			"read assignment after missing-presence GoOnline: %v",
			err,
		)
	}

	if persistedUnassignedAt != nil {
		t.Fatalf(
			"expected assignment to remain active, got unassigned_at=%v",
			*persistedUnassignedAt,
		)
	}
}

func TestHeartbeatSucceedsForBusyOnlineDriver(t *testing.T) {
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

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf("acquire John dispatch fixture lock: %v", err)
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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	// ---------------------------------------------------------
	// Preserve original heartbeat/location state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeading            *float64
		originalSpeed              *float64
		originalAccuracy           *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				heading,
				speed,
				accuracy,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeading,
		&originalSpeed,
		&originalAccuracy,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original heartbeat state: %v",
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
					latitude = $4,
					longitude = $5,
					heading = $6,
					speed = $7,
					accuracy = $8,
					last_heartbeat_at = $9,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeading,
			originalSpeed,
			originalAccuracy,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore heartbeat state: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Put driver online and BUSY.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'BUSY',
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare BUSY heartbeat state: %v",
			err,
		)
	}

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			Config:   cfg,
			Presence: presenceRepo,
		},
	)

	const (
		latitude  = 60.1699
		longitude = 24.9384
		heading   = 180.0
		speed     = 12.5
		accuracy  = 4.0
	)

	beforeHeartbeat := time.Now().UTC()

	err = service.Heartbeat(
		ctx,
		HeartbeatRequest{
			UserID:    johnUserID,
			Latitude:  latitude,
			Longitude: longitude,
			Heading:   heading,
			Speed:     speed,
			Accuracy:  accuracy,
		},
	)

	if err != nil {
		t.Fatalf(
			"heartbeat BUSY online driver: %v",
			err,
		)
	}

	var (
		persistedLatitude  *float64
		persistedLongitude *float64
		persistedHeading   *float64
		persistedSpeed     *float64
		persistedAccuracy  *float64
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				latitude,
				longitude,
				heading,
				speed,
				accuracy,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedLatitude,
		&persistedLongitude,
		&persistedHeading,
		&persistedSpeed,
		&persistedAccuracy,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read heartbeat state: %v",
			err,
		)
	}

	if persistedLatitude == nil ||
		*persistedLatitude != latitude {
		t.Fatalf(
			"expected latitude %v, got %v",
			latitude,
			persistedLatitude,
		)
	}

	if persistedLongitude == nil ||
		*persistedLongitude != longitude {
		t.Fatalf(
			"expected longitude %v, got %v",
			longitude,
			persistedLongitude,
		)
	}

	if persistedHeading == nil ||
		*persistedHeading != heading {
		t.Fatalf(
			"expected heading %v, got %v",
			heading,
			persistedHeading,
		)
	}

	if persistedSpeed == nil ||
		*persistedSpeed != speed {
		t.Fatalf(
			"expected speed %v, got %v",
			speed,
			persistedSpeed,
		)
	}

	if persistedAccuracy == nil ||
		*persistedAccuracy != accuracy {
		t.Fatalf(
			"expected accuracy %v, got %v",
			accuracy,
			persistedAccuracy,
		)
	}

	if persistedHeartbeat == nil {
		t.Fatal(
			"expected last_heartbeat_at to be populated",
		)
	}

	if persistedHeartbeat.Before(
		beforeHeartbeat.Add(-time.Second),
	) {
		t.Fatalf(
			"expected heartbeat to be refreshed, got %v",
			*persistedHeartbeat,
		)
	}
}

func TestHeartbeatRejectsOfflineDriverAndPreservesLocation(t *testing.T) {
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

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf("acquire John dispatch fixture lock: %v", err)
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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	// ---------------------------------------------------------
	// Preserve original state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeading            *float64
		originalSpeed              *float64
		originalAccuracy           *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				heading,
				speed,
				accuracy,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeading,
		&originalSpeed,
		&originalAccuracy,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original offline-heartbeat state: %v",
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
					latitude = $4,
					longitude = $5,
					heading = $6,
					speed = $7,
					accuracy = $8,
					last_heartbeat_at = $9,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeading,
			originalSpeed,
			originalAccuracy,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore offline heartbeat state: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Establish deterministic OFFLINE location/heartbeat state.
	// ---------------------------------------------------------

	const (
		initialLatitude  = 60.1700
		initialLongitude = 24.9400
		initialHeading   = 90.0
		initialSpeed     = 0.0
		initialAccuracy  = 5.0
	)

	initialHeartbeat :=
		time.Now().UTC().Add(-5 * time.Minute)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = FALSE,
				availability_status = 'OFFLINE',
				latitude = $2,
				longitude = $3,
				heading = $4,
				speed = $5,
				accuracy = $6,
				last_heartbeat_at = $7,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
		initialLatitude,
		initialLongitude,
		initialHeading,
		initialSpeed,
		initialAccuracy,
		initialHeartbeat,
	); err != nil {
		t.Fatalf(
			"prepare OFFLINE heartbeat state: %v",
			err,
		)
	}

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			Config:   cfg,
			Presence: presenceRepo,
		},
	)

	err = service.Heartbeat(
		ctx,
		HeartbeatRequest{
			UserID:    johnUserID,
			Latitude:  61.0,
			Longitude: 25.0,
			Heading:   200.0,
			Speed:     30.0,
			Accuracy:  2.0,
		},
	)

	if !errors.Is(
		err,
		ErrDriverHeartbeatUnavailable,
	) {
		t.Fatalf(
			"expected ErrDriverHeartbeatUnavailable, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// Verify rejected heartbeat changed nothing.
	// ---------------------------------------------------------

	var (
		persistedLatitude  *float64
		persistedLongitude *float64
		persistedHeading   *float64
		persistedSpeed     *float64
		persistedAccuracy  *float64
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				latitude,
				longitude,
				heading,
				speed,
				accuracy,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedLatitude,
		&persistedLongitude,
		&persistedHeading,
		&persistedSpeed,
		&persistedAccuracy,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read rejected heartbeat state: %v",
			err,
		)
	}

	if persistedLatitude == nil ||
		*persistedLatitude != initialLatitude {
		t.Fatalf(
			"expected latitude %v to remain unchanged, got %v",
			initialLatitude,
			persistedLatitude,
		)
	}

	if persistedLongitude == nil ||
		*persistedLongitude != initialLongitude {
		t.Fatalf(
			"expected longitude %v to remain unchanged, got %v",
			initialLongitude,
			persistedLongitude,
		)
	}

	if persistedHeading == nil ||
		*persistedHeading != initialHeading {
		t.Fatalf(
			"expected heading %v to remain unchanged, got %v",
			initialHeading,
			persistedHeading,
		)
	}

	if persistedSpeed == nil ||
		*persistedSpeed != initialSpeed {
		t.Fatalf(
			"expected speed %v to remain unchanged, got %v",
			initialSpeed,
			persistedSpeed,
		)
	}

	if persistedAccuracy == nil ||
		*persistedAccuracy != initialAccuracy {
		t.Fatalf(
			"expected accuracy %v to remain unchanged, got %v",
			initialAccuracy,
			persistedAccuracy,
		)
	}

	if persistedHeartbeat == nil {
		t.Fatal(
			"expected original heartbeat timestamp to remain present",
		)
	}

	const timestampTolerance = time.Millisecond

	heartbeatDifference :=
		persistedHeartbeat.Sub(initialHeartbeat)

	if heartbeatDifference < 0 {
		heartbeatDifference = -heartbeatDifference
	}

	if heartbeatDifference > timestampTolerance {
		t.Fatalf(
			"expected heartbeat timestamp %v to remain unchanged, got %v",
			initialHeartbeat,
			*persistedHeartbeat,
		)
	}
}

func TestHeartbeatRejectsInvalidTelemetryAndPreservesState(t *testing.T) {
	ctx := context.Background()

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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	// ---------------------------------------------------------
	// Preserve original state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeading            *float64
		originalSpeed              *float64
		originalAccuracy           *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				heading,
				speed,
				accuracy,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeading,
		&originalSpeed,
		&originalAccuracy,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original invalid-telemetry state: %v",
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
					latitude = $4,
					longitude = $5,
					heading = $6,
					speed = $7,
					accuracy = $8,
					last_heartbeat_at = $9,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeading,
			originalSpeed,
			originalAccuracy,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore invalid-telemetry state: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Establish deterministic valid ONLINE + AVAILABLE state.
	// ---------------------------------------------------------

	const (
		initialLatitude  = 60.1700
		initialLongitude = 24.9400
		initialHeading   = 90.0
		initialSpeed     = 8.0
		initialAccuracy  = 5.0
	)

	initialHeartbeat :=
		time.Now().UTC().Add(-5 * time.Minute)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = $2,
				longitude = $3,
				heading = $4,
				speed = $5,
				accuracy = $6,
				last_heartbeat_at = $7,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
		initialLatitude,
		initialLongitude,
		initialHeading,
		initialSpeed,
		initialAccuracy,
		initialHeartbeat,
	); err != nil {
		t.Fatalf(
			"prepare valid online telemetry state: %v",
			err,
		)
	}

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			Config:   cfg,
			Presence: presenceRepo,
		},
	)

	tests := []struct {
		name          string
		request       HeartbeatRequest
		expectedError error
	}{
		{
			name: "latitude above maximum",
			request: HeartbeatRequest{
				UserID:    johnUserID,
				Latitude:  91,
				Longitude: 24.94,
				Heading:   90,
				Speed:     8,
				Accuracy:  5,
			},
			expectedError: ErrInvalidLatitude,
		},
		{
			name: "longitude above maximum",
			request: HeartbeatRequest{
				UserID:    johnUserID,
				Latitude:  60.17,
				Longitude: 181,
				Heading:   90,
				Speed:     8,
				Accuracy:  5,
			},
			expectedError: ErrInvalidLongitude,
		},
		{
			name: "heading above maximum",
			request: HeartbeatRequest{
				UserID:    johnUserID,
				Latitude:  60.17,
				Longitude: 24.94,
				Heading:   361,
				Speed:     8,
				Accuracy:  5,
			},
			expectedError: ErrInvalidHeading,
		},
		{
			name: "negative speed",
			request: HeartbeatRequest{
				UserID:    johnUserID,
				Latitude:  60.17,
				Longitude: 24.94,
				Heading:   90,
				Speed:     -1,
				Accuracy:  5,
			},
			expectedError: ErrInvalidSpeed,
		},
		{
			name: "negative accuracy",
			request: HeartbeatRequest{
				UserID:    johnUserID,
				Latitude:  60.17,
				Longitude: 24.94,
				Heading:   90,
				Speed:     8,
				Accuracy:  -1,
			},
			expectedError: ErrInvalidAccuracy,
		},
	}

	for _, testCase := range tests {
		t.Run(
			testCase.name,
			func(t *testing.T) {

				err := service.Heartbeat(
					ctx,
					testCase.request,
				)

				if !errors.Is(
					err,
					testCase.expectedError,
				) {
					t.Fatalf(
						"expected %v, got %v",
						testCase.expectedError,
						err,
					)
				}

				var (
					persistedLatitude  *float64
					persistedLongitude *float64
					persistedHeading   *float64
					persistedSpeed     *float64
					persistedAccuracy  *float64
					persistedHeartbeat *time.Time
				)

				if err := db.QueryRow(
					ctx,
					`
						SELECT
							latitude,
							longitude,
							heading,
							speed,
							accuracy,
							last_heartbeat_at
						FROM driver_presence
						WHERE driver_id = $1
					`,
					johnUserID,
				).Scan(
					&persistedLatitude,
					&persistedLongitude,
					&persistedHeading,
					&persistedSpeed,
					&persistedAccuracy,
					&persistedHeartbeat,
				); err != nil {
					t.Fatalf(
						"read telemetry after rejected heartbeat: %v",
						err,
					)
				}

				if persistedLatitude == nil ||
					*persistedLatitude != initialLatitude {
					t.Fatalf(
						"expected latitude %v to remain unchanged, got %v",
						initialLatitude,
						persistedLatitude,
					)
				}

				if persistedLongitude == nil ||
					*persistedLongitude != initialLongitude {
					t.Fatalf(
						"expected longitude %v to remain unchanged, got %v",
						initialLongitude,
						persistedLongitude,
					)
				}

				if persistedHeading == nil ||
					*persistedHeading != initialHeading {
					t.Fatalf(
						"expected heading %v to remain unchanged, got %v",
						initialHeading,
						persistedHeading,
					)
				}

				if persistedSpeed == nil ||
					*persistedSpeed != initialSpeed {
					t.Fatalf(
						"expected speed %v to remain unchanged, got %v",
						initialSpeed,
						persistedSpeed,
					)
				}

				if persistedAccuracy == nil ||
					*persistedAccuracy != initialAccuracy {
					t.Fatalf(
						"expected accuracy %v to remain unchanged, got %v",
						initialAccuracy,
						persistedAccuracy,
					)
				}

				if persistedHeartbeat == nil {
					t.Fatal(
						"expected heartbeat timestamp to remain populated",
					)
				}

				const timestampTolerance = time.Millisecond

				heartbeatDifference :=
					persistedHeartbeat.Sub(initialHeartbeat)

				if heartbeatDifference < 0 {
					heartbeatDifference = -heartbeatDifference
				}

				if heartbeatDifference > timestampTolerance {
					t.Fatalf(
						"expected heartbeat timestamp %v to remain unchanged, got %v",
						initialHeartbeat,
						*persistedHeartbeat,
					)
				}
			},
		)
	}
}
func TestUpdateAvailabilityValidatesManualStatuses(t *testing.T) {
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

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

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

	const johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
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
		&originalIsOnline,
		&originalAvailabilityStatus,
	); err != nil {
		t.Fatalf(
			"load original availability state: %v",
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
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
		); restoreErr != nil {
			t.Logf(
				"restore availability state: %v",
				restoreErr,
			)
		}
	}()

	// Ensure the shared fixture is idle so valid manual transitions
	// are allowed during this test.
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

	presenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	service := NewService(
		Dependencies{
			Config:   cfg,
			Presence: presenceRepo,
		},
	)

	validStatuses := []string{
		StatusAvailable,
		StatusBreak,
		StatusOffDuty,
	}

	for _, status := range validStatuses {
		t.Run(
			"valid_"+status,
			func(t *testing.T) {

				if _, err := db.Exec(
					ctx,
					`
						UPDATE driver_presence
						SET
							is_online = TRUE,
							availability_status = 'AVAILABLE',
							updated_at = NOW()
						WHERE driver_id = $1
					`,
					johnUserID,
				); err != nil {
					t.Fatalf(
						"prepare valid availability state: %v",
						err,
					)
				}

				err := service.UpdateAvailability(
					ctx,
					UpdateAvailabilityRequest{
						UserID: johnUserID,
						Status: status,
					},
				)

				if err != nil {
					t.Fatalf(
						"expected status %s to succeed, got %v",
						status,
						err,
					)
				}

				var persistedStatus string

				if err := db.QueryRow(
					ctx,
					`
						SELECT availability_status
						FROM driver_presence
						WHERE driver_id = $1
					`,
					johnUserID,
				).Scan(
					&persistedStatus,
				); err != nil {
					t.Fatalf(
						"read persisted availability status: %v",
						err,
					)
				}

				if persistedStatus != status {
					t.Fatalf(
						"expected status %s, got %s",
						status,
						persistedStatus,
					)
				}
			},
		)
	}

	invalidStatuses := []string{
		StatusBusy,
		StatusOffline,
		StatusSuspended,
		"INVALID_STATUS",
		"",
	}

	for _, status := range invalidStatuses {
		t.Run(
			"invalid_"+status,
			func(t *testing.T) {

				if _, err := db.Exec(
					ctx,
					`
						UPDATE driver_presence
						SET
							is_online = TRUE,
							availability_status = 'AVAILABLE',
							updated_at = NOW()
						WHERE driver_id = $1
					`,
					johnUserID,
				); err != nil {
					t.Fatalf(
						"prepare invalid availability state: %v",
						err,
					)
				}

				err := service.UpdateAvailability(
					ctx,
					UpdateAvailabilityRequest{
						UserID: johnUserID,
						Status: status,
					},
				)

				if !errors.Is(
					err,
					ErrInvalidAvailabilityStatus,
				) {
					t.Fatalf(
						"expected ErrInvalidAvailabilityStatus for %q, got %v",
						status,
						err,
					)
				}

				var persistedStatus string

				if err := db.QueryRow(
					ctx,
					`
						SELECT availability_status
						FROM driver_presence
						WHERE driver_id = $1
					`,
					johnUserID,
				).Scan(
					&persistedStatus,
				); err != nil {
					t.Fatalf(
						"read availability after rejected status: %v",
						err,
					)
				}

				if persistedStatus != StatusAvailable {
					t.Fatalf(
						"expected rejected status %q to leave AVAILABLE unchanged, got %s",
						status,
						persistedStatus,
					)
				}
			},
		)
	}
}
