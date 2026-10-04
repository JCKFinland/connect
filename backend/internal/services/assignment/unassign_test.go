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

	driverID := driverFixture.UserID

	assignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	activeAssignment, err :=
		assignmentRepo.GetActiveByDriver(
			ctx,
			driverID,
		)
	if err != nil {
		t.Fatalf(
			"load isolated active assignment: %v",
			err,
		)
	}

	if activeAssignment == nil ||
		activeAssignment.ID == "" ||
		activeAssignment.VehicleID == "" {

		t.Fatal(
			"isolated fixture requires an active vehicle assignment",
		)
	}

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
			NOW()
		)
	`,
		driverID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
		driverFixture.VehicleID,
		driverFixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf(
			"create isolated driver presence: %v",
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
			driverID,
		); err != nil {
			t.Logf(
				"cleanup isolated driver presence: %v",
				err,
			)
		}
	}()

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
			        'CANCELLED'
			  )
		`,
		driverID,
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
		driverID,
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
			driverID,
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
		driverID,
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
		driverID,
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
		driverID,
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
		driverID,
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

	driverID := driverFixture.UserID

	assignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	originalAssignment, err :=
		assignmentRepo.GetActiveByDriver(
			ctx,
			driverID,
		)
	if err != nil {
		t.Fatalf(
			"load isolated active assignment: %v",
			err,
		)
	}

	if originalAssignment == nil ||
		originalAssignment.ID == "" ||
		originalAssignment.VehicleID == "" {

		t.Fatal(
			"isolated fixture requires an active vehicle assignment",
		)
	}

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
			NOW()
		)
	`,
		driverID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
		driverFixture.VehicleID,
		driverFixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf(
			"create isolated driver presence: %v",
			err,
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
			        'CANCELLED'
			  )
		`,
		driverID,
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
		driverID,
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
			driverID,
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
		driverID,
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
		driverID,
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
		driverID,
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
		driverID,
		AssignDriverRequest{
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
		driverID,
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
		driverID,
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
		driverID,
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

func TestAssignDerivesAuthenticatedDriverScopeAndRejectsForeignVehicle(
	t *testing.T,
) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Connect using the same integration-test configuration.
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
	// 2. Serialize fixture-backed assignment integration tests.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)
	if err != nil {
		t.Fatalf(
			"acquire assignment fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release assignment fixture lock: %v",
				err,
			)
		}
	}()

	// ---------------------------------------------------------
	// 3. Create two completely isolated organizational scopes.
	// ---------------------------------------------------------

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create authenticated driver fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupDriverFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup authenticated driver fixture: %v",
				err,
			)
		}
	}()

	foreignFixture, cleanupForeignFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create foreign vehicle fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupForeignFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup foreign vehicle fixture: %v",
				err,
			)
		}
	}()

	if driverFixture.CompanyID == foreignFixture.CompanyID {
		t.Fatal(
			"authority regression requires distinct companies",
		)
	}

	if driverFixture.BranchID == foreignFixture.BranchID {
		t.Fatal(
			"authority regression requires distinct branches",
		)
	}

	assignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	// ---------------------------------------------------------
	// 4. Close fixture A's initial assignment so its authenticated
	//    user can exercise the real Assign workflow.
	// ---------------------------------------------------------

	if err := assignmentRepo.CloseAssignment(
		ctx,
		driverFixture.AssignmentID,
	); err != nil {
		t.Fatalf(
			"close authenticated driver's fixture assignment: %v",
			err,
		)
	}

	// Assign requires the driver's presence row as its lifecycle
	// serialization point. Start it idle and unassigned.
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
				last_heartbeat_at
			)
			VALUES (
				$1,
				$2,
				$3,
				NULL,
				NULL,
				FALSE,
				'AVAILABLE',
				NOW()
			)
		`,
		driverFixture.UserID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
	)
	if err != nil {
		t.Fatalf(
			"create idle authenticated driver presence: %v",
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
			driverFixture.UserID,
		); err != nil {
			t.Logf(
				"cleanup authenticated driver presence: %v",
				err,
			)
		}
	}()

	service := NewService(
		Dependencies{
			DB:          db,
			Assignments: assignmentRepo,
		},
	)

	// ---------------------------------------------------------
	// 5. Authenticated driver must not be able to select a
	//    vehicle belonging to another company/branch.
	// ---------------------------------------------------------

	var assignmentCountBefore int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
		`,
		driverFixture.UserID,
	).Scan(&assignmentCountBefore); err != nil {
		t.Fatalf(
			"count assignments before foreign vehicle attempt: %v",
			err,
		)
	}

	createdAssignment, err := service.Assign(
		ctx,
		driverFixture.UserID,
		AssignDriverRequest{
			VehicleID: foreignFixture.VehicleID,
			Notes:     "must reject foreign organizational scope",
		},
	)

	if !errors.Is(
		err,
		ErrVehicleOutsideDriverScope,
	) {
		t.Fatalf(
			"expected ErrVehicleOutsideDriverScope, got %v",
			err,
		)
	}

	if createdAssignment != nil {
		t.Fatalf(
			"expected no foreign-scope assignment, got %+v",
			createdAssignment,
		)
	}

	var assignmentCountAfterRejected int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
		`,
		driverFixture.UserID,
	).Scan(&assignmentCountAfterRejected); err != nil {
		t.Fatalf(
			"count assignments after foreign vehicle attempt: %v",
			err,
		)
	}

	if assignmentCountAfterRejected != assignmentCountBefore {
		t.Fatalf(
			"foreign vehicle attempt changed assignment history: before=%d after=%d",
			assignmentCountBefore,
			assignmentCountAfterRejected,
		)
	}

	// ---------------------------------------------------------
	// 6. An inactive vehicle remains administratively visible but
	//    cannot acquire a new active assignment.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE vehicles
			SET is_active = FALSE,
			    updated_at = NOW()
			WHERE id = $1
		`,
		driverFixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"deactivate assignment vehicle for regression: %v",
			err,
		)
	}

	var assignmentCountBeforeInactiveAttempt int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
		`,
		driverFixture.UserID,
	).Scan(&assignmentCountBeforeInactiveAttempt); err != nil {
		t.Fatalf(
			"count assignments before inactive vehicle attempt: %v",
			err,
		)
	}

	createdAssignment, err = service.Assign(
		ctx,
		driverFixture.UserID,
		AssignDriverRequest{
			VehicleID: driverFixture.VehicleID,
			Notes:     "must reject inactive vehicle",
		},
	)

	if !errors.Is(
		err,
		ErrVehicleInactive,
	) {
		t.Fatalf(
			"expected ErrVehicleInactive, got %v",
			err,
		)
	}

	if createdAssignment != nil {
		t.Fatalf(
			"expected no inactive-vehicle assignment, got %+v",
			createdAssignment,
		)
	}

	var assignmentCountAfterInactiveAttempt int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
		`,
		driverFixture.UserID,
	).Scan(&assignmentCountAfterInactiveAttempt); err != nil {
		t.Fatalf(
			"count assignments after inactive vehicle attempt: %v",
			err,
		)
	}

	if assignmentCountAfterInactiveAttempt !=
		assignmentCountBeforeInactiveAttempt {

		t.Fatalf(
			"inactive vehicle attempt changed assignment history: before=%d after=%d",
			assignmentCountBeforeInactiveAttempt,
			assignmentCountAfterInactiveAttempt,
		)
	}

	if _, err := db.Exec(
		ctx,
		`
			UPDATE vehicles
			SET is_active = TRUE,
			    updated_at = NOW()
			WHERE id = $1
		`,
		driverFixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"reactivate assignment vehicle after regression: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. The driver's own active vehicle must succeed, with every
	//    relationship persisted from authoritative server data.
	// ---------------------------------------------------------

	createdAssignment, err = service.Assign(
		ctx,
		driverFixture.UserID,
		AssignDriverRequest{
			VehicleID: driverFixture.VehicleID,
			Notes:     "authoritative relationship regression",
		},
	)
	if err != nil {
		t.Fatalf(
			"assign authoritative vehicle: %v",
			err,
		)
	}

	if createdAssignment == nil ||
		createdAssignment.ID == "" {
		t.Fatal(
			"expected created authoritative assignment",
		)
	}

	// The successful assignment is additional history beyond the fixture's
	// original assignment. Remove it before fixture cleanup so its RESTRICT
	// vehicle foreign key cannot strand the disposable hierarchy.
	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_assignments
				WHERE id = $1
			`,
			createdAssignment.ID,
		); err != nil {
			t.Logf(
				"cleanup authoritative assignment: %v",
				err,
			)
		}
	}()

	if createdAssignment.DriverID != driverFixture.UserID {
		t.Fatalf(
			"created driver identity mismatch: got %q want %q",
			createdAssignment.DriverID,
			driverFixture.UserID,
		)
	}

	if createdAssignment.CompanyID != driverFixture.CompanyID {
		t.Fatalf(
			"created company mismatch: got %q want %q",
			createdAssignment.CompanyID,
			driverFixture.CompanyID,
		)
	}

	if createdAssignment.BranchID != driverFixture.BranchID {
		t.Fatalf(
			"created branch mismatch: got %q want %q",
			createdAssignment.BranchID,
			driverFixture.BranchID,
		)
	}

	if createdAssignment.FleetID != driverFixture.FleetID {
		t.Fatalf(
			"created fleet mismatch: got %q want %q",
			createdAssignment.FleetID,
			driverFixture.FleetID,
		)
	}

	if createdAssignment.VehicleID != driverFixture.VehicleID {
		t.Fatalf(
			"created vehicle mismatch: got %q want %q",
			createdAssignment.VehicleID,
			driverFixture.VehicleID,
		)
	}

	var (
		persistedCompanyID string
		persistedBranchID  string
		persistedFleetID   string
		persistedDriverID  string
		persistedVehicleID string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				company_id,
				branch_id,
				fleet_id,
				driver_id,
				vehicle_id
			FROM driver_assignments
			WHERE id = $1
		`,
		createdAssignment.ID,
	).Scan(
		&persistedCompanyID,
		&persistedBranchID,
		&persistedFleetID,
		&persistedDriverID,
		&persistedVehicleID,
	); err != nil {
		t.Fatalf(
			"load persisted authoritative assignment: %v",
			err,
		)
	}

	if persistedCompanyID != driverFixture.CompanyID ||
		persistedBranchID != driverFixture.BranchID ||
		persistedFleetID != driverFixture.FleetID ||
		persistedDriverID != driverFixture.UserID ||
		persistedVehicleID != driverFixture.VehicleID {

		t.Fatalf(
			"persisted assignment relationships are not authoritative: company=%q branch=%q fleet=%q driver=%q vehicle=%q",
			persistedCompanyID,
			persistedBranchID,
			persistedFleetID,
			persistedDriverID,
			persistedVehicleID,
		)
	}
}
