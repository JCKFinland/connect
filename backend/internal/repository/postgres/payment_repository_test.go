package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestPaymentRepositoryCreateFromCompletedTrip(t *testing.T) {
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
		t.Fatalf("acquire shared fixture lock: %v", err)
	}

	defer func() {
		if err := releaseFixtureLock(context.Background()); err != nil {
			t.Logf("release shared fixture lock: %v", err)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"
		driverID   = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	var (
		companyID string
		branchID  string
		fleetID   string
		vehicleID string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				company_id,
				branch_id,
				fleet_id,
				vehicle_id
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
			LIMIT 1
		`,
		driverID,
	).Scan(
		&companyID,
		&branchID,
		&fleetID,
		&vehicleID,
	)
	if err != nil {
		t.Fatalf("resolve active driver assignment: %v", err)
	}

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()
	fareID := uuid.NewString()

	now := time.Now().UTC()

	_, err = db.Exec(
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
				'Payment Repository Test Pickup',
				60.2055,
				24.6559,
				'Payment Repository Test Destination',
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
		rideRequestID,
		customerID,
		now,
		now.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("create payment test ride request: %v", err)
	}

	_, err = db.Exec(
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
				actual_distance_meters,
				actual_duration_seconds,
				assigned_at,
				started_at,
				completed_at,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				'COMPLETED',
				8420,
				900,
				$9,
				$10,
				$11,
				$9,
				$11
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		now.Add(-20*time.Minute),
		now.Add(-15*time.Minute),
		now,
	)
	if err != nil {
		t.Fatalf("create payment test trip: %v", err)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trip_fares (
				id,
				trip_id,
				base_fare,
				distance_fare,
				time_fare,
				waiting_fare,
				booking_fee,
				surge_multiplier,
				surge_amount,
				discount_amount,
				tax_amount,
				toll_amount,
				parking_amount,
				total_amount,
				currency,
				distance_rate_per_km,
				time_rate_per_minute,
				waiting_rate_per_minute,
				charged_distance_meters,
				charged_duration_seconds,
				waiting_duration_seconds,
				pricing_version,
				calculated_at,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				4.90,
				12.63,
				3.75,
				1.25,
				2.00,
				1.00,
				0,
				0,
				5.17,
				0,
				0,
				29.70,
				'EUR',
				1.5000,
				0.2500,
				0.2500,
				8420,
				900,
				300,
				'payment-test-v1',
				$3,
				$3,
				$3
			)
		`,
		fareID,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf("create payment test fare: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM payments WHERE trip_id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup payment: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf("cleanup ride request: %v", cleanupErr)
		}
	}()

	repo := NewPaymentRepository(db)

	payment, err := repo.CreateFromCompletedTrip(
		ctx,
		tripID,
		"CASH",
	)
	if err != nil {
		t.Fatalf("create payment from completed trip: %v", err)
	}

	if payment.ID == "" {
		t.Fatal("expected payment ID")
	}

	if payment.TripID != tripID {
		t.Fatalf(
			"trip ID mismatch: got %s want %s",
			payment.TripID,
			tripID,
		)
	}

	if payment.FareID != fareID {
		t.Fatalf(
			"fare ID mismatch: got %s want %s",
			payment.FareID,
			fareID,
		)
	}

	if payment.CustomerID != customerID {
		t.Fatalf(
			"customer ID mismatch: got %s want %s",
			payment.CustomerID,
			customerID,
		)
	}

	if payment.Status != "PENDING" {
		t.Fatalf(
			"status mismatch: got %s want PENDING",
			payment.Status,
		)
	}

	if payment.PaymentMethod != "CASH" {
		t.Fatalf(
			"payment method mismatch: got %s want CASH",
			payment.PaymentMethod,
		)
	}

	if payment.Amount != "29.70" {
		t.Fatalf(
			"amount mismatch: got %s want 29.70",
			payment.Amount,
		)
	}

	if payment.Currency != "EUR" {
		t.Fatalf(
			"currency mismatch: got %s want EUR",
			payment.Currency,
		)
	}

	byID, err := repo.GetByID(ctx, payment.ID)
	if err != nil {
		t.Fatalf("get payment by ID: %v", err)
	}

	if byID.ID != payment.ID {
		t.Fatalf(
			"retrieved payment ID mismatch: got %s want %s",
			byID.ID,
			payment.ID,
		)
	}

	byTripID, err := repo.GetByTripID(ctx, tripID)
	if err != nil {
		t.Fatalf("get payment by trip ID: %v", err)
	}

	if byTripID.ID != payment.ID {
		t.Fatalf(
			"trip payment ID mismatch: got %s want %s",
			byTripID.ID,
			payment.ID,
		)
	}

	_, err = repo.CreateFromCompletedTrip(
		ctx,
		tripID,
		"CASH",
	)
	if err == nil {
		t.Fatal("expected duplicate payment creation to fail")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf(
			"expected PostgreSQL constraint error, got: %v",
			err,
		)
	}

	if pgErr.Code != "23505" {
		t.Fatalf(
			"expected unique violation 23505, got %s: %v",
			pgErr.Code,
			err,
		)
	}
}
