package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestDriverPresenceRepositoryTripLifecycleAuthority(t *testing.T) {
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
	t.Cleanup(func() {
		db.Close()
	})

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanupFixture(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	arrivedAt := now.Add(time.Second)
	startedAt := arrivedAt.Add(time.Second)
	completedAt := startedAt.Add(time.Second)
	cancelledAt := completedAt.Add(time.Second)

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
			VALUES ($1, $2, $3, $4, $5, TRUE, 'AVAILABLE', $6)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("create driver presence: %v", err)
	}
	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		); cleanupErr != nil {
			t.Logf("cleanup driver presence: %v", cleanupErr)
		}
	}()

	createRideRequest := func() string {
		t.Helper()

		id := uuid.NewString()

		_, err := db.Exec(
			ctx,
			`
				INSERT INTO ride_requests (
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
				VALUES (
					$1,
					$2,
					'Presence Lifecycle Test Pickup',
					60.2055,
					24.6559,
					'Presence Lifecycle Test Destination',
					60.1719,
					24.9414,
					'STANDARD',
					1,
					'ACCEPTED',
					$3,
					$4,
					$3,
					$3
				)
			`,
			id,
			fixture.UserID,
			now,
			now.Add(10*time.Minute),
		)
		if err != nil {
			t.Fatalf("create ride request: %v", err)
		}

		t.Cleanup(func() {
			if _, cleanupErr := db.Exec(
				context.Background(),
				`DELETE FROM ride_requests WHERE id = $1`,
				id,
			); cleanupErr != nil {
				t.Logf("cleanup ride request %s: %v", id, cleanupErr)
			}
		})

		return id
	}

	createTrip := func(
		rideRequestID string,
		status string,
		isActive bool,
	) string {
		t.Helper()

		id := uuid.NewString()

		_, err := db.Exec(
			ctx,
			`
				INSERT INTO trips (
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
				VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8,
					$9, $10, $11, $10, $10
				)
			`,
			id,
			rideRequestID,
			fixture.UserID,
			fixture.UserID,
			fixture.VehicleID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.FleetID,
			status,
			now,
			isActive,
		)
		if err != nil {
			t.Fatalf("create %s trip: %v", status, err)
		}

		t.Cleanup(func() {
			if _, cleanupErr := db.Exec(
				context.Background(),
				`DELETE FROM trips WHERE id = $1`,
				id,
			); cleanupErr != nil {
				t.Logf("cleanup trip %s: %v", id, cleanupErr)
			}
		})

		return id
	}

	readPresence := func() (bool, string) {
		t.Helper()

		var (
			isOnline bool
			status   string
		)

		err := db.QueryRow(
			ctx,
			`
				SELECT is_online, availability_status
				FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		).Scan(&isOnline, &status)
		if err != nil {
			t.Fatalf("read driver presence: %v", err)
		}

		return isOnline, status
	}

	repo := NewDriverPresenceRepository(db)

	// No exact active trip means no claim.
	claimed, err := repo.ClaimForTrip(
		ctx,
		fixture.UserID,
		uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("claim with unknown trip: %v", err)
	}
	if claimed {
		t.Fatal("expected unknown trip not to authorize BUSY claim")
	}

	isOnline, status := readPresence()
	if !isOnline || status != "AVAILABLE" {
		t.Fatalf(
			"expected AVAILABLE online presence after rejected claim, got status=%q online=%v",
			status,
			isOnline,
		)
	}

	// Exact active non-terminal trip authorizes AVAILABLE -> BUSY.
	firstRequestID := createRideRequest()
	firstTripID := createTrip(firstRequestID, "ASSIGNED", true)

	claimed, err = repo.ClaimForTrip(
		ctx,
		fixture.UserID,
		firstTripID,
	)
	if err != nil {
		t.Fatalf("claim active trip: %v", err)
	}
	if !claimed {
		t.Fatal("expected active trip to authorize BUSY claim")
	}

	isOnline, status = readPresence()
	if !isOnline || status != "BUSY" {
		t.Fatalf(
			"expected BUSY online presence after claim, got status=%q online=%v",
			status,
			isOnline,
		)
	}

	// An active trip cannot authorize release.
	released, err := repo.ReleaseFromTrip(
		ctx,
		fixture.UserID,
		firstTripID,
	)
	if err != nil {
		t.Fatalf("release active trip: %v", err)
	}
	if released {
		t.Fatal("expected active trip not to authorize release")
	}

	// Terminalize the exact trip with a valid completed lifecycle shape.
	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET
				status = 'COMPLETED',
				is_active = FALSE,
				driver_arrived_at = $2,
				started_at = $3,
				completed_at = $4,
				updated_at = $4
			WHERE id = $1
		`,
		firstTripID,
		arrivedAt,
		startedAt,
		completedAt,
	)
	if err != nil {
		t.Fatalf("terminalize first trip: %v", err)
	}

	// Another active trip must still prevent release.
	secondRequestID := createRideRequest()
	secondTripID := createTrip(secondRequestID, "ASSIGNED", true)

	released, err = repo.ReleaseFromTrip(
		ctx,
		fixture.UserID,
		firstTripID,
	)
	if err != nil {
		t.Fatalf("release with another active trip: %v", err)
	}
	if released {
		t.Fatal("expected another active trip to prevent release")
	}

	isOnline, status = readPresence()
	if !isOnline || status != "BUSY" {
		t.Fatalf(
			"expected BUSY online presence while another trip is active, got status=%q online=%v",
			status,
			isOnline,
		)
	}

	// Once no active trip remains, the exact terminal trip may release presence.
	// ASSIGNED -> CANCELLED legitimately has no arrival/start timestamps.
	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET
				status = 'CANCELLED',
				is_active = FALSE,
				cancelled_at = $2,
				updated_at = $2
			WHERE id = $1
		`,
		secondTripID,
		cancelledAt,
	)
	if err != nil {
		t.Fatalf("terminalize second trip: %v", err)
	}

	released, err = repo.ReleaseFromTrip(
		ctx,
		fixture.UserID,
		firstTripID,
	)
	if err != nil {
		t.Fatalf("release terminal trip: %v", err)
	}
	if !released {
		t.Fatal("expected terminal trip to authorize release")
	}

	isOnline, status = readPresence()
	if !isOnline || status != "AVAILABLE" {
		t.Fatalf(
			"expected AVAILABLE online presence after release, got status=%q online=%v",
			status,
			isOnline,
		)
	}

	// A terminal trip cannot manufacture BUSY from AVAILABLE.
	claimed, err = repo.ClaimForTrip(
		ctx,
		fixture.UserID,
		firstTripID,
	)
	if err != nil {
		t.Fatalf("claim terminal trip: %v", err)
	}
	if claimed {
		t.Fatal("expected terminal trip not to authorize BUSY claim")
	}
}
