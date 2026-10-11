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

func TestTripRepositoryGetIDByRideRequestID(t *testing.T) {
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

	releaseLock, err := testutil.AcquirePostgresFixtureLock(
		ctx, db, "dispatch-fixture:john",
	)
	if err != nil {
		t.Fatalf("acquire fixture lock: %v", err)
	}
	defer func() {
		if err := releaseLock(context.Background()); err != nil {
			t.Logf("release fixture lock: %v", err)
		}
	}()

	driver, cleanupDriver, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin test transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	repo := NewTripRepositoryWithDB(tx)

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"
	requestID := uuid.NewString()
	tripID := uuid.NewString()
	now := time.Now().UTC()

	_, err = tx.Exec(ctx, `
		INSERT INTO ride_requests (
			id, customer_id,
			pickup_address, pickup_latitude, pickup_longitude,
			destination_address, destination_latitude,
			destination_longitude, requested_vehicle_type,
			passenger_count, status, requested_at,
			expires_at, created_at, updated_at
		) VALUES (
			$1, $2,
			'Discovery Test Pickup', 60.169856, 24.938379,
			'Discovery Test Destination', 60.170500,
			24.940000, 'STANDARD',
			1, 'ACCEPTED', $3,
			$4, $3, $3
		)
	`, requestID, customerID, now, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("insert ride request: %v", err)
	}

	t.Run("no trip yet", func(t *testing.T) {
		result, err := repo.GetIDByRideRequestID(ctx, requestID)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected pgx.ErrNoRows, got id=%q err=%v", result, err)
		}
	})

	_, err = tx.Exec(ctx, `
		INSERT INTO trips (
			id, ride_request_id, customer_id,
			driver_id, vehicle_id, company_id,
			branch_id, fleet_id, status,
			assigned_at, is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, 'ASSIGNED',
			$9, TRUE, $9, $9
		)
	`,
		tripID, requestID, customerID,
		driver.UserID, driver.VehicleID, driver.CompanyID,
		driver.BranchID, driver.FleetID, now,
	)
	if err != nil {
		t.Fatalf("insert assigned trip: %v", err)
	}

	t.Run("existing trip", func(t *testing.T) {
		result, err := repo.GetIDByRideRequestID(ctx, requestID)
		if err != nil {
			t.Fatalf("resolve trip: %v", err)
		}
		if result != tripID {
			t.Fatalf("expected %q, got %q", tripID, result)
		}
	})

	_, err = tx.Exec(ctx, `
		UPDATE trips
		SET deleted_at = $2, updated_at = $2
		WHERE id = $1
	`, tripID, now)
	if err != nil {
		t.Fatalf("soft-delete trip: %v", err)
	}

	t.Run("soft-deleted trip", func(t *testing.T) {
		result, err := repo.GetIDByRideRequestID(ctx, requestID)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected pgx.ErrNoRows, got id=%q err=%v", result, err)
		}
	})
}
