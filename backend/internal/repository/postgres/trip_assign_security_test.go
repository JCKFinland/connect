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
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTripAssignmentTenantSecurity(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("refusing assignment security test outside integration database")
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	release, err := testutil.AcquirePostgresFixtureLock(
		ctx, db, "dispatch-fixture:john",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := release(context.Background()); err != nil {
			t.Errorf("release fixture lock: %v", err)
		}
	}()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup customer: %v", err)
		}
	}()

	companyA, cleanupA, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupA(context.Background()); err != nil {
			t.Errorf("cleanup company A: %v", err)
		}
	}()

	companyB, cleanupB, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupB(context.Background()); err != nil {
			t.Errorf("cleanup company B: %v", err)
		}
	}()

	repo := NewTripRepository(db)

	type testTrip struct {
		id        string
		requestID string
	}

	var created []testTrip

	defer func() {
		for _, item := range created {
			if _, err := db.Exec(
				context.Background(),
				`DELETE FROM trips WHERE id = $1`,
				item.id,
			); err != nil {
				t.Errorf("cleanup trip: %v", err)
			}

			if _, err := db.Exec(
				context.Background(),
				`DELETE FROM ride_requests WHERE id = $1`,
				item.requestID,
			); err != nil {
				t.Errorf("cleanup request: %v", err)
			}
		}
	}()

	createTrip := func() string {
		t.Helper()

		requestID := uuid.NewString()
		tripID := uuid.NewString()
		now := time.Now().UTC()

		_, err := db.Exec(ctx, `
            INSERT INTO ride_requests (
                id, customer_id,
                pickup_address, pickup_latitude, pickup_longitude,
                destination_address, destination_latitude,
                destination_longitude, requested_vehicle_type,
                passenger_count, status, requested_at,
                expires_at, created_at, updated_at
            )
            VALUES (
                $1, $2,
                'Assignment Security Pickup', 60.17, 24.94,
                'Assignment Security Destination', 60.18, 24.95,
                'STANDARD', 1, 'ACCEPTED',
                $3, $4, $3, $3
            )
        `, requestID, customerID, now, now.Add(time.Hour))
		if err != nil {
			t.Fatalf("create ride request: %v", err)
		}

		created = append(created, testTrip{
			id: tripID, requestID: requestID,
		})

		trip := &models.Trip{
			RideRequestID: requestID,
			CustomerID:    customerID,
			DriverID:      companyA.UserID,
			VehicleID:     companyA.VehicleID,
			CompanyID:     companyA.CompanyID,
			BranchID:      companyA.BranchID,
			FleetID:       companyA.FleetID,
			AssignedAt:    now,
		}
		trip.ID = tripID

		if err := repo.Create(ctx, trip); err != nil {
			t.Fatalf("create assigned trip: %v", err)
		}

		return tripID
	}

	assertUnchanged := func(tripID string) {
		t.Helper()

		var driverID, vehicleID, status string

		err := db.QueryRow(ctx, `
            SELECT driver_id, vehicle_id, status
            FROM trips
            WHERE id = $1
        `, tripID).Scan(&driverID, &vehicleID, &status)
		if err != nil {
			t.Fatal(err)
		}

		if driverID != companyA.UserID ||
			vehicleID != companyA.VehicleID ||
			status != "ASSIGNED" {
			t.Fatalf(
				"trip mutated after rejection: driver=%s vehicle=%s status=%s",
				driverID, vehicleID, status,
			)
		}
	}

	t.Run("valid same-company assignment", func(t *testing.T) {
		tripID := createTrip()

		err := repo.AssignDriver(
			ctx, tripID, companyA.UserID, companyA.VehicleID,
		)
		if err != nil {
			t.Fatalf("valid assignment rejected: %v", err)
		}

		assertUnchanged(tripID)
	})

	t.Run("cross-company driver rejected", func(t *testing.T) {
		tripID := createTrip()

		err := repo.AssignDriver(
			ctx, tripID, companyB.UserID, companyA.VehicleID,
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected rejection, got %v", err)
		}

		assertUnchanged(tripID)
	})

	t.Run("cross-company vehicle rejected", func(t *testing.T) {
		tripID := createTrip()

		err := repo.AssignDriver(
			ctx, tripID, companyA.UserID, companyB.VehicleID,
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected rejection, got %v", err)
		}

		assertUnchanged(tripID)
	})

	t.Run("inactive assignment rejected", func(t *testing.T) {
		tripID := createTrip()

		_, err := db.Exec(ctx, `
            UPDATE driver_assignments
            SET unassigned_at = NOW()
            WHERE id = $1
        `, companyA.AssignmentID)
		if err != nil {
			t.Fatalf("deactivate assignment: %v", err)
		}

		err = repo.AssignDriver(
			ctx, tripID, companyA.UserID, companyA.VehicleID,
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected inactive assignment rejection, got %v", err)
		}

		assertUnchanged(tripID)
	})
}
