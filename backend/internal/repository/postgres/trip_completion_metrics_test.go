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

func TestTripRepositoryUpdateActualMetricsRequiresActiveInProgressTrip(
	t *testing.T,
) {
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

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated driver fixture: %v", err)
	}
	defer func() {
		if err := cleanupDriverFixture(context.Background()); err != nil {
			t.Logf("cleanup isolated driver fixture: %v", err)
		}
	}()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

	now := time.Now().UTC().Truncate(time.Microsecond)
	assignedAt := now.Add(-20 * time.Minute)
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
				'Trip Metrics Test Pickup',
				60.169856,
				24.938379,
				'Trip Metrics Test Destination',
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
		t.Fatalf("create trip metrics test ride request: %v", err)
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
				$10,
				TRUE,
				$9,
				$10
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverFixture.UserID,
		driverFixture.VehicleID,
		driverFixture.CompanyID,
		driverFixture.BranchID,
		driverFixture.FleetID,
		assignedAt,
		startedAt,
	)
	if err != nil {
		t.Fatalf("create trip metrics test trip: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup trip metrics test trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup trip metrics test ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewTripRepository(db)

	const (
		authoritativeDistance = int64(12500)
		authoritativeDuration = int64(1080)
	)

	if err := repo.UpdateActualMetrics(
		ctx,
		tripID,
		authoritativeDistance,
		authoritativeDuration,
	); err != nil {
		t.Fatalf(
			"update actual metrics for active IN_PROGRESS trip: %v",
			err,
		)
	}

	var (
		persistedDistance *int64
		persistedDuration *int64
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				actual_distance_meters,
				actual_duration_seconds
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&persistedDistance,
		&persistedDuration,
	)
	if err != nil {
		t.Fatalf("read persisted actual metrics: %v", err)
	}

	if persistedDistance == nil ||
		*persistedDistance != authoritativeDistance {
		t.Fatalf(
			"expected actual_distance_meters %d, got %v",
			authoritativeDistance,
			persistedDistance,
		)
	}

	if persistedDuration == nil ||
		*persistedDuration != authoritativeDuration {
		t.Fatalf(
			"expected actual_duration_seconds %d, got %v",
			authoritativeDuration,
			persistedDuration,
		)
	}

	completedAt := now.Add(time.Minute)

	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET
				status = 'COMPLETED',
				completed_at = $2,
				is_active = FALSE,
				updated_at = $2
			WHERE id = $1
		`,
		tripID,
		completedAt,
	)
	if err != nil {
		t.Fatalf("transition trip fixture to COMPLETED: %v", err)
	}

	err = repo.UpdateActualMetrics(
		ctx,
		tripID,
		99999,
		9999,
	)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf(
			"expected pgx.ErrNoRows when rewriting completed trip metrics, got %v",
			err,
		)
	}

	persistedDistance = nil
	persistedDuration = nil

	err = db.QueryRow(
		ctx,
		`
			SELECT
				actual_distance_meters,
				actual_duration_seconds
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&persistedDistance,
		&persistedDuration,
	)
	if err != nil {
		t.Fatalf("read actual metrics after rejected rewrite: %v", err)
	}

	if persistedDistance == nil ||
		*persistedDistance != authoritativeDistance {
		t.Fatalf(
			"expected rejected rewrite to preserve actual_distance_meters %d, got %v",
			authoritativeDistance,
			persistedDistance,
		)
	}

	if persistedDuration == nil ||
		*persistedDuration != authoritativeDuration {
		t.Fatalf(
			"expected rejected rewrite to preserve actual_duration_seconds %d, got %v",
			authoritativeDuration,
			persistedDuration,
		)
	}
}
