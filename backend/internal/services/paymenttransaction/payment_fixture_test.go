package paymenttransaction

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

type paymentTransactionTestFixture struct {
	DB *pgxpool.Pool

	Payment *models.Payment

	PaymentID     string
	TripID        string
	RideRequestID string
}

func newPaymentTransactionTestFixture(
	t *testing.T,
) *paymentTransactionTestFixture {
	t.Helper()

	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	cfg, err := config.Load()

	_ = os.Chdir(originalDir)

	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)
	if err != nil {
		db.Close()

		t.Fatalf(
			"acquire fixture lock: %v",
			err,
		)
	}

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
		_ = releaseFixtureLock(ctx)
		db.Close()

		t.Fatalf(
			"resolve active driver assignment: %v",
			err,
		)
	}

	now := time.Now().UTC()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				'Payment Operation Test Pickup',
				60.2055,
				24.6559,
				'Payment Operation Test Destination',
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
		_ = releaseFixtureLock(ctx)
		db.Close()

		t.Fatalf(
			"create payment operation ride request: %v",
			err,
		)
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
				assigned_at,
				started_at,
				completed_at,
				is_active,
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
				$9,
				$10,
				$11,
				FALSE,
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
		now.Add(-10*time.Minute),
		now,
	)
	if err != nil {
		_ = releaseFixtureLock(ctx)
		db.Close()

		t.Fatalf(
			"create payment operation trip: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trip_fares (
				trip_id,
				base_fare,
				total_amount,
				currency,
				surge_multiplier,
				pricing_version,
				calculated_at
			)
			VALUES (
				$1,
				1.00,
				1.00,
				'EUR',
				1.00,
				'payment-operation-test-v1',
				$2
			)
		`,
		tripID,
		now,
	)
	if err != nil {
		_ = releaseFixtureLock(ctx)
		db.Close()

		t.Fatalf(
			"create payment operation fare: %v",
			err,
		)
	}

	paymentRepo :=
		postgresrepo.NewPaymentRepository(db)

	payment, err :=
		paymentRepo.CreateFromCompletedTrip(
			ctx,
			tripID,
			"CARD",
		)
	if err != nil {
		_ = releaseFixtureLock(ctx)
		db.Close()

		t.Fatalf(
			"create payment operation payment: %v",
			err,
		)
	}

	fixture := &paymentTransactionTestFixture{
		DB: db,

		Payment: payment,

		PaymentID:     payment.ID,
		TripID:        tripID,
		RideRequestID: rideRequestID,
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM payments WHERE id = $1`,
			fixture.PaymentID,
		); err != nil {
			t.Logf(
				"cleanup payment operation payment: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM trips WHERE id = $1`,
			fixture.TripID,
		); err != nil {
			t.Logf(
				"cleanup payment operation trip: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			fixture.RideRequestID,
		); err != nil {
			t.Logf(
				"cleanup payment operation ride request: %v",
				err,
			)
		}

		if err := releaseFixtureLock(
			cleanupCtx,
		); err != nil {
			t.Logf(
				"release payment operation fixture lock: %v",
				err,
			)
		}

		db.Close()
	})

	return fixture
}
