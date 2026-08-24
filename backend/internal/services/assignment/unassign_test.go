package assignment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestUnassignRejectsActiveTripAndPreservesAssignmentState(t *testing.T) {
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
	// 2. Serialize access to John's shared dispatch fixture.
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
	// 3. Avoid interfering with an existing real active trip.
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
	// 4. Preserve presence state.
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

	if originalAssignmentID == nil {
		t.Fatal(
			"expected driver presence assignment_id to be populated",
		)
	}

	if *originalAssignmentID != activeAssignment.ID {
		t.Fatalf(
			"presence assignment %s does not match active assignment %s",
			*originalAssignmentID,
			activeAssignment.ID,
		)
	}

	if originalVehicleID == nil {
		t.Fatal(
			"expected driver presence vehicle_id to be populated",
		)
	}

	// Restore shared fixture even if this test fails.
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
	// 5. Mark driver BUSY.
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
	// 6. Create disposable ACCEPTED ride request.
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
				'Unassign Guard Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'Transactional unassignment guard integration test',
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
			"create unassignment guard ride request: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Create active ASSIGNED trip using the real assignment
	//    ownership values.
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
			"create active unassignment guard trip: %v",
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
				"cleanup unassignment guard trip: %v",
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
				"cleanup unassignment guard ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 8. Construct real assignment service.
	// ---------------------------------------------------------

	service := NewService(
		Dependencies{
			DB:          db,
			Assignments: assignmentRepo,
		},
	)

	// ---------------------------------------------------------
	// 9. Unassignment must be rejected.
	// ---------------------------------------------------------

	err = service.Unassign(
		ctx,
		UnassignDriverRequest{
			DriverID: johnUserID,
		},
	)

	if !errors.Is(
		err,
		presence.ErrDriverAvailabilityLocked,
	) {
		t.Fatalf(
			"expected ErrDriverAvailabilityLocked, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Assignment must still be active.
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
			"read assignment after rejected unassignment: %v",
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
	// 11. Presence relationship must remain intact.
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
			"read presence after rejected unassignment: %v",
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

	if persistedAvailabilityStatus != presence.StatusBusy {
		t.Fatalf(
			"expected driver status %s, got %s",
			presence.StatusBusy,
			persistedAvailabilityStatus,
		)
	}
}

func TestAssignRejectsActiveTripAndRollsBackNewAssignment(t *testing.T) {
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
	// 2. Serialize John's shared integration fixture.
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

	assignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	originalAssignment, err :=
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

	if originalAssignment == nil ||
		originalAssignment.ID == "" ||
		originalAssignment.VehicleID == "" {

		t.Fatal(
			"John fixture requires an active vehicle assignment",
		)
	}

	// ---------------------------------------------------------
	// 3. Do not interfere with an existing real active trip.
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
	// 4. Preserve presence state.
	// ---------------------------------------------------------

	var (
		originalPresenceAssignmentID *string
		originalPresenceVehicleID    *string
		originalIsOnline             bool
		originalAvailabilityStatus   string
		originalHeartbeat            *time.Time
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
		&originalPresenceAssignmentID,
		&originalPresenceVehicleID,
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	if originalPresenceAssignmentID == nil ||
		*originalPresenceAssignmentID != originalAssignment.ID {

		t.Fatal(
			"John presence must reference his active assignment",
		)
	}

	if originalPresenceVehicleID == nil ||
		*originalPresenceVehicleID != originalAssignment.VehicleID {

		t.Fatal(
			"John presence must reference his assigned vehicle",
		)
	}

	// ---------------------------------------------------------
	// 5. Always restore John's shared fixture.
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
			originalAssignment.ID,
		); restoreErr != nil {
			t.Logf(
				"restore original assignment: %v",
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
			originalPresenceAssignmentID,
			originalPresenceVehicleID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore original presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 6. Fixture preparation:
	//
	// Temporarily close the original assignment so Assign()
	// reaches the guarded attachment path.
	//
	// Presence deliberately remains attached to the original
	// assignment because we want to prove failed Assign() does
	// not overwrite it.
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
		originalAssignment.ID,
	); err != nil {
		t.Fatalf(
			"temporarily close original assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Mark John BUSY.
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
	// 8. Create disposable ACCEPTED ride request.
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
				'Assign Rollback Guard Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'Transactional assignment rollback integration test',
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
			"create assignment rollback ride: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Create active trip using John's original operational
	//    assignment snapshot.
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
		originalAssignment.VehicleID,
		originalAssignment.CompanyID,
		originalAssignment.BranchID,
		originalAssignment.FleetID,
		now,
	)

	if err != nil {
		t.Fatalf(
			"create active assignment rollback trip: %v",
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
				"cleanup assignment rollback trip: %v",
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
				"cleanup assignment rollback ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 10. Record assignment history count before attempted
	//     replacement assignment.
	// ---------------------------------------------------------

	var assignmentCountBefore int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&assignmentCountBefore,
	); err != nil {
		t.Fatalf(
			"count assignments before rejected assign: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. Construct real assignment service.
	// ---------------------------------------------------------

	service := NewService(
		Dependencies{
			DB:          db,
			Assignments: assignmentRepo,
		},
	)

	// ---------------------------------------------------------
	// 12. Replacement assignment must be rejected because John
	//     is BUSY and committed to an active trip.
	//
	// We deliberately reuse the original vehicle. Its assignment
	// was temporarily closed, so the active-vehicle uniqueness
	// check does not prevent reaching the lifecycle guard.
	// ---------------------------------------------------------

	createdAssignment, err := service.Assign(
		ctx,
		AssignDriverRequest{
			CompanyID: originalAssignment.CompanyID,
			BranchID:  originalAssignment.BranchID,
			FleetID:   originalAssignment.FleetID,
			DriverID:  johnUserID,
			VehicleID: originalAssignment.VehicleID,
			Notes:     "must roll back because driver has active trip",
		},
	)

	if !errors.Is(
		err,
		presence.ErrDriverAvailabilityLocked,
	) {
		t.Fatalf(
			"expected ErrDriverAvailabilityLocked, got %v",
			err,
		)
	}

	if createdAssignment != nil {
		t.Fatalf(
			"expected no created assignment, got %+v",
			createdAssignment,
		)
	}

	// ---------------------------------------------------------
	// 13. The INSERT inside Assign() must have rolled back.
	// ---------------------------------------------------------

	var assignmentCountAfter int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&assignmentCountAfter,
	); err != nil {
		t.Fatalf(
			"count assignments after rejected assign: %v",
			err,
		)
	}

	if assignmentCountAfter != assignmentCountBefore {
		t.Fatalf(
			"expected assignment count to remain %d, got %d",
			assignmentCountBefore,
			assignmentCountAfter,
		)
	}

	// ---------------------------------------------------------
	// 14. There must be no new active assignment.
	// ---------------------------------------------------------

	var activeAssignmentCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
		`,
		johnUserID,
	).Scan(
		&activeAssignmentCount,
	); err != nil {
		t.Fatalf(
			"count active assignments after rejected assign: %v",
			err,
		)
	}

	if activeAssignmentCount != 0 {
		t.Fatalf(
			"expected zero active assignments after rejected assign, got %d",
			activeAssignmentCount,
		)
	}

	// ---------------------------------------------------------
	// 15. Presence must remain attached to the original fixture
	//     state and remain BUSY/online.
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
			"read presence after rejected assign: %v",
			err,
		)
	}

	if persistedAssignmentID == nil ||
		*persistedAssignmentID != originalAssignment.ID {

		t.Fatalf(
			"expected original assignment_id %s, got %v",
			originalAssignment.ID,
			persistedAssignmentID,
		)
	}

	if persistedVehicleID == nil ||
		*persistedVehicleID != originalAssignment.VehicleID {

		t.Fatalf(
			"expected original vehicle_id %s, got %v",
			originalAssignment.VehicleID,
			persistedVehicleID,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected BUSY driver to remain online",
		)
	}

	if persistedAvailabilityStatus != presence.StatusBusy {
		t.Fatalf(
			"expected driver status %s, got %s",
			presence.StatusBusy,
			persistedAvailabilityStatus,
		)
	}
}
