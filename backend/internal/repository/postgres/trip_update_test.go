package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTripRepositoryUpdatePreservesLifecycleFields(t *testing.T) {
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
				'Trip Update Test Pickup',
				60.169856,
				24.938379,
				'Trip Update Test Destination',
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
		t.Fatalf("create trip update test ride request: %v", err)
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
				actual_distance_km,
				actual_duration_minutes,
				actual_distance_meters,
				actual_duration_seconds,
				is_active,
				passenger_note,
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
				12.5,
				18,
				12500,
				1080,
				TRUE,
				'original note',
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
		t.Fatalf("create trip update test trip: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup trip update test trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup trip update test ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewTripRepository(db)

	trip, err := repo.GetByID(ctx, tripID)
	if err != nil {
		t.Fatalf("get trip before update: %v", err)
	}

	updatedNote := "updated editable note"
	updatedPickup := "Updated Trip Pickup"

	// Deliberately mutate lifecycle-controlled fields in memory. Generic
	// Update must ignore all of them.
	completedAt := now.Add(time.Minute)
	cancelledAt := now.Add(2 * time.Minute)
	cancelledBy := driverFixture.UserID
	cancellationReason := "must not persist"

	trip.Status = "COMPLETED"
	trip.AssignedAt = now.Add(-time.Hour)
	trip.StartedAt = nil
	trip.CompletedAt = &completedAt
	trip.CancelledAt = &cancelledAt
	trip.CancelledBy = &cancelledBy
	trip.CancellationReason = &cancellationReason
	trip.IsActive = false

	// Actual trip metrics are completion evidence and must not be writable
	// through the generic Update path.
	poisonDistanceKM := 99.9
	poisonDurationMinutes := 99
	poisonDistanceMeters := int64(99999)
	poisonDurationSeconds := int64(9999)

	trip.ActualDistanceKM = &poisonDistanceKM
	trip.ActualDurationMinutes = &poisonDurationMinutes
	trip.ActualDistanceMeters = &poisonDistanceMeters
	trip.ActualDurationSeconds = &poisonDurationSeconds

	// Editable fields must still be persisted.
	trip.PassengerNote = &updatedNote
	trip.PickupAddress = &updatedPickup

	if err := repo.Update(ctx, trip); err != nil {
		t.Fatalf("update editable trip fields: %v", err)
	}

	var (
		persistedStatus             string
		persistedAssignedAt         time.Time
		persistedStartedAt          *time.Time
		persistedCompletedAt        *time.Time
		persistedCancelledAt        *time.Time
		persistedCancelledBy        *string
		persistedCancellationReason *string
		persistedIsActive           bool
		persistedActualDistanceKM   *float64
		persistedActualDurationMin  *int
		persistedActualDistanceM    *int64
		persistedActualDurationSec  *int64
		persistedPassengerNote      *string
		persistedPickupAddress      *string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				assigned_at,
				started_at,
				completed_at,
				cancelled_at,
				cancelled_by,
				cancellation_reason,
				is_active,
				actual_distance_km,
				actual_duration_minutes,
				actual_distance_meters,
				actual_duration_seconds,
				passenger_note,
				pickup_address
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&persistedStatus,
		&persistedAssignedAt,
		&persistedStartedAt,
		&persistedCompletedAt,
		&persistedCancelledAt,
		&persistedCancelledBy,
		&persistedCancellationReason,
		&persistedIsActive,
		&persistedActualDistanceKM,
		&persistedActualDurationMin,
		&persistedActualDistanceM,
		&persistedActualDurationSec,
		&persistedPassengerNote,
		&persistedPickupAddress,
	)
	if err != nil {
		t.Fatalf("read trip after update: %v", err)
	}

	if persistedStatus != "IN_PROGRESS" {
		t.Fatalf(
			"expected status IN_PROGRESS to be preserved, got %s",
			persistedStatus,
		)
	}

	if !persistedIsActive {
		t.Fatal("expected is_active=true to be preserved")
	}

	if !persistedAssignedAt.Equal(assignedAt) {
		t.Fatalf(
			"expected assigned_at %s to be preserved, got %s",
			assignedAt.Format(time.RFC3339Nano),
			persistedAssignedAt.Format(time.RFC3339Nano),
		)
	}

	if persistedStartedAt == nil ||
		!persistedStartedAt.Equal(startedAt) {
		t.Fatalf(
			"expected started_at %s to be preserved, got %v",
			startedAt.Format(time.RFC3339Nano),
			persistedStartedAt,
		)
	}

	if persistedCompletedAt != nil {
		t.Fatalf(
			"expected completed_at to remain NULL, got %s",
			persistedCompletedAt.Format(time.RFC3339Nano),
		)
	}

	if persistedCancelledAt != nil {
		t.Fatalf(
			"expected cancelled_at to remain NULL, got %s",
			persistedCancelledAt.Format(time.RFC3339Nano),
		)
	}

	if persistedCancelledBy != nil {
		t.Fatalf(
			"expected cancelled_by to remain NULL, got %q",
			*persistedCancelledBy,
		)
	}

	if persistedCancellationReason != nil {
		t.Fatalf(
			"expected cancellation_reason to remain NULL, got %q",
			*persistedCancellationReason,
		)
	}

	if persistedActualDistanceKM == nil ||
		*persistedActualDistanceKM != 12.5 {
		t.Fatalf(
			"expected actual_distance_km 12.5 to be preserved, got %v",
			persistedActualDistanceKM,
		)
	}

	if persistedActualDurationMin == nil ||
		*persistedActualDurationMin != 18 {
		t.Fatalf(
			"expected actual_duration_minutes 18 to be preserved, got %v",
			persistedActualDurationMin,
		)
	}

	if persistedActualDistanceM == nil ||
		*persistedActualDistanceM != 12500 {
		t.Fatalf(
			"expected actual_distance_meters 12500 to be preserved, got %v",
			persistedActualDistanceM,
		)
	}

	if persistedActualDurationSec == nil ||
		*persistedActualDurationSec != 1080 {
		t.Fatalf(
			"expected actual_duration_seconds 1080 to be preserved, got %v",
			persistedActualDurationSec,
		)
	}

	if persistedPassengerNote == nil ||
		*persistedPassengerNote != updatedNote {
		t.Fatalf(
			"expected passenger_note %q, got %v",
			updatedNote,
			persistedPassengerNote,
		)
	}

	if persistedPickupAddress == nil ||
		*persistedPickupAddress != updatedPickup {
		t.Fatalf(
			"expected pickup_address %q, got %v",
			updatedPickup,
			persistedPickupAddress,
		)
	}
}
