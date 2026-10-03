package trip

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestCreateCanonicalizesInitialLifecycleState(t *testing.T) {
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
				'Trip Create Authority Test Pickup',
				60.169856,
				24.938379,
				'Trip Create Authority Test Destination',
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
		t.Fatalf("create ride request fixture: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup create test trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf("cleanup create test ride request: %v", cleanupErr)
		}
	}()

	progressedAt := now.Add(-5 * time.Minute)
	actualDistanceKM := 12.5
	actualDurationMinutes := 18
	actualDistanceMeters := int64(12500)
	actualDurationSeconds := int64(1080)
	cancelledBy := driver.UserID
	cancellationReason := "caller-supplied lifecycle evidence"

	newTrip := &models.Trip{
		RideRequestID: rideRequestID,
		CustomerID:    customerID,
		DriverID:      driver.UserID,
		VehicleID:     driver.VehicleID,
		CompanyID:     driver.CompanyID,
		BranchID:      driver.BranchID,
		FleetID:       driver.FleetID,

		Status:     StatusCompleted,
		AssignedAt: progressedAt,

		DriverArrivedAt: &progressedAt,
		StartedAt:       &progressedAt,
		CompletedAt:     &progressedAt,
		CancelledAt:     &progressedAt,

		ActualDistanceKM:      &actualDistanceKM,
		ActualDurationMinutes: &actualDurationMinutes,
		ActualDistanceMeters:  &actualDistanceMeters,
		ActualDurationSeconds: &actualDurationSeconds,

		CancelledBy:        &cancelledBy,
		CancellationReason: &cancellationReason,

		IsActive: false,
	}

	service := NewService(
		Dependencies{
			Trips: postgresrepo.NewTripRepository(db),
		},
	)

	beforeCreate := time.Now().UTC()

	if err := service.Create(ctx, newTrip); err != nil {
		t.Fatalf("create trip: %v", err)
	}

	afterCreate := time.Now().UTC()

	// Create must canonicalize the caller-visible model as well as persistence.
	if newTrip.Status != StatusAssigned {
		t.Fatalf(
			"expected caller model status ASSIGNED, got %s",
			newTrip.Status,
		)
	}

	if newTrip.ID == "" {
		t.Fatal("expected Create to assign trip ID")
	}

	tripID = newTrip.ID

	if newTrip.AssignedAt.Before(beforeCreate) ||
		newTrip.AssignedAt.After(afterCreate) {
		t.Fatalf(
			"expected fresh assigned_at within Create call, got %v",
			newTrip.AssignedAt,
		)
	}

	if newTrip.DriverArrivedAt != nil ||
		newTrip.StartedAt != nil ||
		newTrip.CompletedAt != nil ||
		newTrip.CancelledAt != nil {
		t.Fatal(
			"expected progressed operational timestamps to be cleared",
		)
	}

	if newTrip.ActualDistanceKM != nil ||
		newTrip.ActualDurationMinutes != nil ||
		newTrip.ActualDistanceMeters != nil ||
		newTrip.ActualDurationSeconds != nil {
		t.Fatal("expected actual trip metrics to be cleared")
	}

	if newTrip.CancelledBy != nil ||
		newTrip.CancellationReason != nil {
		t.Fatal(
			"expected cancellation metadata to be cleared",
		)
	}

	if !newTrip.IsActive {
		t.Fatal("expected newly created trip to be active")
	}

	var (
		persistedStatus                string
		persistedAssignedAt            time.Time
		persistedDriverArrivedAt       *time.Time
		persistedStartedAt             *time.Time
		persistedCompletedAt           *time.Time
		persistedCancelledAt           *time.Time
		persistedActualDistanceKM      *float64
		persistedActualDurationMinutes *int
		persistedActualDistanceMeters  *int64
		persistedActualDurationSeconds *int64
		persistedCancelledBy           *string
		persistedCancelReason          *string
		persistedIsActive              bool
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
				actual_distance_km,
				actual_duration_minutes,
				actual_distance_meters,
				actual_duration_seconds,
				cancelled_by,
				cancellation_reason,
				is_active
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
		&persistedActualDistanceKM,
		&persistedActualDurationMinutes,
		&persistedActualDistanceMeters,
		&persistedActualDurationSeconds,
		&persistedCancelledBy,
		&persistedCancelReason,
		&persistedIsActive,
	)
	if err != nil {
		t.Fatalf("load created trip: %v", err)
	}

	if persistedStatus != StatusAssigned {
		t.Fatalf(
			"expected persisted status ASSIGNED, got %s",
			persistedStatus,
		)
	}

	assignedAtDelta := persistedAssignedAt.Sub(newTrip.AssignedAt)
	if assignedAtDelta < 0 {
		assignedAtDelta = -assignedAtDelta
	}

	if assignedAtDelta >= time.Microsecond {
		t.Fatalf(
			"expected persisted assigned_at to represent the same instant within PostgreSQL precision; model=%v persisted=%v delta=%v",
			newTrip.AssignedAt,
			persistedAssignedAt,
			assignedAtDelta,
		)
	}

	if persistedDriverArrivedAt != nil ||
		persistedStartedAt != nil ||
		persistedCompletedAt != nil ||
		persistedCancelledAt != nil {
		t.Fatal(
			"expected persisted progressed operational timestamps to be NULL",
		)
	}

	if persistedActualDistanceKM != nil ||
		persistedActualDurationMinutes != nil ||
		persistedActualDistanceMeters != nil ||
		persistedActualDurationSeconds != nil {
		t.Fatal("expected persisted actual trip metrics to be NULL")
	}

	if persistedCancelledBy != nil ||
		persistedCancelReason != nil {
		t.Fatal(
			"expected persisted cancellation metadata to be NULL",
		)
	}

	if !persistedIsActive {
		t.Fatal("expected persisted trip to be active")
	}
}
