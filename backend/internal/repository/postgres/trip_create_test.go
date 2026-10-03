package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTripRepositoryCreateCanonicalizesInitialLifecycle(t *testing.T) {
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

	releaseFixtureLock, err := testutil.AcquirePostgresFixtureLock(
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
	assignedAt := now.Add(-30 * time.Minute)
	driverArrivedAt := now.Add(-20 * time.Minute)
	startedAt := now.Add(-15 * time.Minute)
	completedAt := now.Add(-5 * time.Minute)
	cancelledAt := now.Add(-4 * time.Minute)

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
				'Trip Create Test Pickup',
				60.169856,
				24.938379,
				'Trip Create Test Destination',
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
		t.Fatalf("create trip create test ride request: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup trip create test trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf("cleanup trip create test ride request: %v", cleanupErr)
		}
	}()

	repo := NewTripRepository(db)

	if err := repo.Create(ctx, nil); err == nil {
		t.Fatal("expected nil trip creation to fail")
	}

	zeroAssignedAtTrip := &models.Trip{}
	if err := repo.Create(ctx, zeroAssignedAtTrip); err == nil {
		t.Fatal("expected zero assigned_at creation to fail")
	}

	legacyDistance := 91.25
	legacyDuration := 777
	actualDistanceMeters := int64(91250)
	actualDurationSeconds := int64(46620)
	cancelledBy := driverFixture.UserID
	cancellationReason := "poisoned cancellation evidence"

	trip := &models.Trip{
		BaseModel: models.BaseModel{
			ID: tripID,
		},

		RideRequestID: rideRequestID,
		CustomerID:    customerID,

		DriverID:  driverFixture.UserID,
		VehicleID: driverFixture.VehicleID,
		FleetID:   driverFixture.FleetID,

		CompanyID: driverFixture.CompanyID,
		BranchID:  driverFixture.BranchID,

		Status: "COMPLETED",

		ActualDistanceKM:      &legacyDistance,
		ActualDurationMinutes: &legacyDuration,
		ActualDistanceMeters:  &actualDistanceMeters,
		ActualDurationSeconds: &actualDurationSeconds,

		AssignedAt:      assignedAt,
		DriverArrivedAt: &driverArrivedAt,
		StartedAt:       &startedAt,
		CompletedAt:     &completedAt,
		CancelledAt:     &cancelledAt,

		CancelledBy:        &cancelledBy,
		CancellationReason: &cancellationReason,

		IsActive: false,
	}

	if err := repo.Create(ctx, trip); err != nil {
		t.Fatalf("create trip with poisoned lifecycle evidence: %v", err)
	}

	if trip.Status != "ASSIGNED" {
		t.Fatalf("expected in-memory status ASSIGNED, got %q", trip.Status)
	}

	if !trip.IsActive {
		t.Fatal("expected in-memory trip to be active")
	}

	if trip.DriverArrivedAt != nil ||
		trip.StartedAt != nil ||
		trip.CompletedAt != nil ||
		trip.CancelledAt != nil {
		t.Fatalf(
			"expected in-memory progressed lifecycle timestamps cleared, got arrived=%v started=%v completed=%v cancelled=%v",
			trip.DriverArrivedAt,
			trip.StartedAt,
			trip.CompletedAt,
			trip.CancelledAt,
		)
	}

	if trip.CancelledBy != nil || trip.CancellationReason != nil {
		t.Fatalf(
			"expected in-memory cancellation evidence cleared, got cancelled_by=%v reason=%v",
			trip.CancelledBy,
			trip.CancellationReason,
		)
	}

	if trip.ActualDistanceKM != nil ||
		trip.ActualDurationMinutes != nil ||
		trip.ActualDistanceMeters != nil ||
		trip.ActualDurationSeconds != nil {
		t.Fatalf(
			"expected in-memory actual metrics cleared, got km=%v minutes=%v meters=%v seconds=%v",
			trip.ActualDistanceKM,
			trip.ActualDurationMinutes,
			trip.ActualDistanceMeters,
			trip.ActualDurationSeconds,
		)
	}

	var (
		persistedStatus                string
		persistedAssignedAt            time.Time
		persistedDriverArrivedAt       *time.Time
		persistedStartedAt             *time.Time
		persistedCompletedAt           *time.Time
		persistedCancelledAt           *time.Time
		persistedCancelledBy           *string
		persistedCancellationReason    *string
		persistedIsActive              bool
		persistedActualDistanceKM      *float64
		persistedActualDurationMinutes *int
		persistedActualDistanceMeters  *int64
		persistedActualDurationSeconds *int64
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				assigned_at,
				driver_arrived_at,
				started_at,
				completed_at,
				cancelled_at,
				cancelled_by,
				cancellation_reason,
				is_active,
				actual_distance_km,
				actual_duration_minutes,
				actual_distance_meters,
				actual_duration_seconds
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&persistedStatus,
		&persistedAssignedAt,
		&persistedDriverArrivedAt,
		&persistedStartedAt,
		&persistedCompletedAt,
		&persistedCancelledAt,
		&persistedCancelledBy,
		&persistedCancellationReason,
		&persistedIsActive,
		&persistedActualDistanceKM,
		&persistedActualDurationMinutes,
		&persistedActualDistanceMeters,
		&persistedActualDurationSeconds,
	)
	if err != nil {
		t.Fatalf("read persisted canonical trip: %v", err)
	}

	if persistedStatus != "ASSIGNED" {
		t.Fatalf("expected persisted status ASSIGNED, got %q", persistedStatus)
	}

	if !persistedAssignedAt.Equal(assignedAt) {
		t.Fatalf(
			"expected persisted assigned_at %v, got %v",
			assignedAt,
			persistedAssignedAt,
		)
	}

	if !persistedIsActive {
		t.Fatal("expected persisted trip to be active")
	}

	if persistedDriverArrivedAt != nil ||
		persistedStartedAt != nil ||
		persistedCompletedAt != nil ||
		persistedCancelledAt != nil {
		t.Fatalf(
			"expected persisted progressed lifecycle timestamps NULL, got arrived=%v started=%v completed=%v cancelled=%v",
			persistedDriverArrivedAt,
			persistedStartedAt,
			persistedCompletedAt,
			persistedCancelledAt,
		)
	}

	if persistedCancelledBy != nil || persistedCancellationReason != nil {
		t.Fatalf(
			"expected persisted cancellation evidence NULL, got cancelled_by=%v reason=%v",
			persistedCancelledBy,
			persistedCancellationReason,
		)
	}

	if persistedActualDistanceKM != nil ||
		persistedActualDurationMinutes != nil ||
		persistedActualDistanceMeters != nil ||
		persistedActualDurationSeconds != nil {
		t.Fatalf(
			"expected persisted actual metrics NULL, got km=%v minutes=%v meters=%v seconds=%v",
			persistedActualDistanceKM,
			persistedActualDurationMinutes,
			persistedActualDistanceMeters,
			persistedActualDurationSeconds,
		)
	}
}
