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

func TestTripAssignmentSameCompanyReassignment(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("refusing test outside integration database")
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

	original, cleanupOriginal, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupOriginal(context.Background()); err != nil {
			t.Errorf("cleanup original driver: %v", err)
		}
	}()

	replacementUserID := uuid.NewString()
	replacementDriverID := uuid.NewString()
	replacementVehicleID := uuid.NewString()
	replacementAssignmentID := uuid.NewString()

	key := uuid.NewString()
	now := time.Now().UTC()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	_, err = tx.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash,
            first_name, last_name, phone
        )
        VALUES (
            $1, $2, 'test-fixture-password-hash',
            'Replacement', 'Driver', $3
        )
    `,
		replacementUserID,
		"replacement-"+key+"@example.test",
		"+358000000004",
	)
	if err != nil {
		t.Fatalf("create replacement user: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO vehicles (
            id, company_id, branch_id, fleet_id,
            registration_number, vin, make, model,
            model_year, color, vehicle_type,
            seating_capacity, is_active
        )
        VALUES (
            $1, $2, $3, $4,
            $5, $6, 'CONNECT', 'Replacement Vehicle',
            2026, 'Black', 'SEDAN', 4, TRUE
        )
    `,
		replacementVehicleID,
		original.CompanyID,
		original.BranchID,
		original.FleetID,
		"REP-"+key[:8],
		"VIN"+key[:8],
	)
	if err != nil {
		t.Fatalf("create replacement vehicle: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO drivers (
            id, user_id, company_id, branch_id,
            driver_number, first_name, last_name,
            phone, email,
            taxi_driver_license_number,
            driving_license_number,
            driving_license_expiry,
            hire_date, status, is_verified,
            is_active, created_at, updated_at
        )
        VALUES (
            $1, $2, $3, $4,
            $5, 'Replacement', 'Driver',
            $6, $7, $8, $9, $10,
            $11, 'ACTIVE', TRUE,
            TRUE, $12, $12
        )
    `,
		replacementDriverID,
		replacementUserID,
		original.CompanyID,
		original.BranchID,
		"DRV-"+key,
		"+358000000004",
		"replacement-"+key+"@example.test",
		"TAXI-"+key,
		"LIC-"+key,
		now.AddDate(5, 0, 0),
		now,
		now,
	)
	if err != nil {
		t.Fatalf("create replacement driver: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO driver_assignments (
            id, company_id, branch_id, fleet_id,
            driver_id, vehicle_id, assigned_at, notes
        )
        VALUES (
            $1, $2, $3, $4,
            $5, $6, $7,
            'Disposable same-company reassignment test'
        )
    `,
		replacementAssignmentID,
		original.CompanyID,
		original.BranchID,
		original.FleetID,
		replacementUserID,
		replacementVehicleID,
		now,
	)
	if err != nil {
		t.Fatalf("create replacement assignment: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit replacement fixtures: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		cleanupTx, err := db.Begin(cleanupCtx)
		if err != nil {
			t.Errorf("begin replacement cleanup: %v", err)
			return
		}
		defer cleanupTx.Rollback(cleanupCtx)

		operations := []struct {
			query string
			id    string
		}{
			{`DELETE FROM driver_assignments WHERE id = $1`, replacementAssignmentID},
			{`DELETE FROM drivers WHERE id = $1`, replacementDriverID},
			{`DELETE FROM vehicles WHERE id = $1`, replacementVehicleID},
			{`DELETE FROM users WHERE id = $1`, replacementUserID},
		}

		for _, operation := range operations {
			if _, err := cleanupTx.Exec(
				cleanupCtx, operation.query, operation.id,
			); err != nil {
				t.Errorf("cleanup replacement fixture: %v", err)
				return
			}
		}

		if err := cleanupTx.Commit(cleanupCtx); err != nil {
			t.Errorf("commit replacement cleanup: %v", err)
		}
	}()

	requestID := uuid.NewString()
	tripID := uuid.NewString()

	_, err = db.Exec(ctx, `
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
            'Reassignment Pickup', 60.17, 24.94,
            'Reassignment Destination', 60.18, 24.95,
            'STANDARD', 1, 'ACCEPTED',
            $3, $4, $3, $3
        )
    `, requestID, customerID, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			requestID,
		); err != nil {
			t.Errorf("cleanup ride request: %v", err)
		}
	}()

	repo := NewTripRepository(db)

	trip := &models.Trip{
		RideRequestID: requestID,
		CustomerID:    customerID,
		DriverID:      original.UserID,
		VehicleID:     original.VehicleID,
		CompanyID:     original.CompanyID,
		BranchID:      original.BranchID,
		FleetID:       original.FleetID,
		AssignedAt:    now,
	}
	trip.ID = tripID

	if err := repo.Create(ctx, trip); err != nil {
		t.Fatalf("create assigned trip: %v", err)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	}()

	if err := repo.AssignDriver(
		ctx,
		tripID,
		replacementUserID,
		replacementVehicleID,
	); err != nil {
		t.Fatalf("same-company reassignment rejected: %v", err)
	}

	var persistedDriverID, persistedVehicleID, status string

	err = db.QueryRow(ctx, `
        SELECT driver_id, vehicle_id, status
        FROM trips
        WHERE id = $1
    `, tripID).Scan(
		&persistedDriverID,
		&persistedVehicleID,
		&status,
	)
	if err != nil {
		t.Fatalf("read reassigned trip: %v", err)
	}

	if persistedDriverID != replacementUserID {
		t.Fatalf(
			"expected replacement driver %s, got %s",
			replacementUserID,
			persistedDriverID,
		)
	}

	if persistedVehicleID != replacementVehicleID {
		t.Fatalf(
			"expected replacement vehicle %s, got %s",
			replacementVehicleID,
			persistedVehicleID,
		)
	}

	if status != "ASSIGNED" {
		t.Fatalf("expected ASSIGNED status, got %s", status)
	}
}
