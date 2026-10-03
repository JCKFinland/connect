package trip

import (
	"context"
	"errors"
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

func TestDeleteAuthorizedRequiresTerminalTrip(t *testing.T) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	// ---------------------------------------------------------
	// 3. Serialize access to the shared integration fixture.
	// ---------------------------------------------------------

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
			t.Logf("release John dispatch fixture lock: %v", err)
		}
	}()

	// ---------------------------------------------------------
	// 4. Controlled existing fixture IDs.
	//
	// trips.driver_id uses users.id.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"

		systemAdminUserID = "ac03d190-c1fb-44b7-a7b5-08e3a43b29af"
	)

	// ---------------------------------------------------------
	// 5. Construct the real service dependencies used by
	//    DeleteAuthorized.
	// ---------------------------------------------------------

	service := NewService(
		Dependencies{
			DB:        db,
			Trips:     postgresrepo.NewTripRepository(db),
			UserRoles: repository.NewUserRoleRepository(db),
		},
	)

	// ---------------------------------------------------------
	// 6. Create disposable ride request and IN_PROGRESS trip.
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
				'Trip Delete Authority Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				'Terminal-only trip deletion regression',
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
		t.Fatalf("create delete test ride request: %v", err)
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
				driver_arrived_at,
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
		t.Fatalf("create delete test trip: %v", err)
	}

	// ---------------------------------------------------------
	// 7. Cleanup disposable records.
	//
	// Use direct SQL so cleanup remains possible even if the
	// behavior under test rejects service-level deletion.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trip_events
				WHERE trip_id = $1
			`,
			tripID,
		); err != nil {
			t.Logf("cleanup trip events: %v", err)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); err != nil {
			t.Logf("cleanup trip: %v", err)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); err != nil {
			t.Logf("cleanup ride request: %v", err)
		}
	}()

	// ---------------------------------------------------------
	// 8. Nonterminal deletion must be rejected.
	// ---------------------------------------------------------

	err = service.DeleteAuthorized(
		ctx,
		tripID,
		systemAdminUserID,
	)
	if !errors.Is(
		err,
		ErrTripDeleteRequiresTerminalStatus,
	) {
		t.Fatalf(
			"expected terminal-status deletion rejection, got: %v",
			err,
		)
	}

	var (
		status    string
		isActive  bool
		deletedAt *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				status,
				is_active,
				deleted_at
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&status,
		&isActive,
		&deletedAt,
	); err != nil {
		t.Fatalf(
			"load trip after rejected deletion: %v",
			err,
		)
	}

	if status != StatusInProgress {
		t.Fatalf(
			"expected IN_PROGRESS after rejected deletion, got %s",
			status,
		)
	}

	if !isActive {
		t.Fatal(
			"expected trip to remain active after rejected deletion",
		)
	}

	if deletedAt != nil {
		t.Fatalf(
			"expected deleted_at to remain NULL after rejected deletion, got %v",
			deletedAt,
		)
	}

	// ---------------------------------------------------------
	// 9. Make the fixture lifecycle-valid CANCELLED.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET
				status = 'CANCELLED',
				cancelled_at = $2,
				is_active = FALSE,
				updated_at = $2
			WHERE id = $1
		`,
		tripID,
		now.Add(time.Second),
	)
	if err != nil {
		t.Fatalf(
			"prepare cancelled trip for deletion: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Terminal deletion must succeed.
	// ---------------------------------------------------------

	if err := service.DeleteAuthorized(
		ctx,
		tripID,
		systemAdminUserID,
	); err != nil {
		t.Fatalf(
			"delete cancelled trip: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 11. Deletion archives the terminal lifecycle state.
	// ---------------------------------------------------------

	status = ""
	isActive = true
	deletedAt = nil

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				status,
				is_active,
				deleted_at
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&status,
		&isActive,
		&deletedAt,
	); err != nil {
		t.Fatalf(
			"load terminal trip after deletion: %v",
			err,
		)
	}

	if status != StatusCancelled {
		t.Fatalf(
			"expected CANCELLED lifecycle state to be preserved, got %s",
			status,
		)
	}

	if isActive {
		t.Fatal(
			"expected deleted terminal trip to remain inactive",
		)
	}

	if deletedAt == nil {
		t.Fatal(
			"expected deleted_at to be set for terminal trip",
		)
	}
}
