package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTripRepositoryDeleteRejectsNonterminalTrip(t *testing.T) {
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
		if err := releaseFixtureLock(context.Background()); err != nil {
			t.Logf("release John dispatch fixture lock: %v", err)
		}
	}()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	driver, cleanupDriver, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

	now := time.Now().UTC().Truncate(time.Microsecond)
	assignedAt := now.Add(-30 * time.Minute)
	arrivedAt := now.Add(-20 * time.Minute)
	startedAt := now.Add(-10 * time.Minute)

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
				requested_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Trip Delete Repository Test Pickup',
				60.169856,
				24.938379,
				'Trip Delete Repository Test Destination',
				60.170500,
				24.940000,
				'STANDARD',
				1,
				'ACCEPTED',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("create trip delete test ride request: %v", err)
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
				$10,
				$11,
				TRUE,
				$9,
				$11
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driver.UserID,
		driver.VehicleID,
		driver.CompanyID,
		driver.BranchID,
		driver.FleetID,
		assignedAt,
		arrivedAt,
		startedAt,
	)
	if err != nil {
		t.Fatalf("create nonterminal trip: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup trip delete test trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup trip delete test ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewTripRepository(db)

	err = repo.Delete(
		ctx,
		tripID,
	)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf(
			"expected pgx.ErrNoRows deleting nonterminal trip, got %v",
			err,
		)
	}

	var (
		persistedStatus    string
		persistedIsActive  bool
		persistedDeletedAt *time.Time
	)

	err = db.QueryRow(
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
		&persistedStatus,
		&persistedIsActive,
		&persistedDeletedAt,
	)
	if err != nil {
		t.Fatalf(
			"load trip after rejected repository deletion: %v",
			err,
		)
	}

	if persistedStatus != "IN_PROGRESS" {
		t.Fatalf(
			"expected status IN_PROGRESS, got %s",
			persistedStatus,
		)
	}

	if !persistedIsActive {
		t.Fatal(
			"expected nonterminal trip to remain active",
		)
	}

	if persistedDeletedAt != nil {
		t.Fatalf(
			"expected deleted_at to remain NULL, got %v",
			persistedDeletedAt,
		)
	}
}
