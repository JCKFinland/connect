package payment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestPaymentLifecycleSerializesAndPersistsPaidState(t *testing.T) {
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
		t.Fatalf("acquire fixture lock: %v", err)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf("release fixture lock: %v", err)
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
				'Payment Lifecycle Test Pickup',
				60.2055,
				24.6559,
				'Payment Lifecycle Test Destination',
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
		t.Fatalf(
			"create lifecycle ride request: %v",
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
				5000,
				600,
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
		now.Add(-10*time.Minute),
		now,
	)
	if err != nil {
		t.Fatalf(
			"create lifecycle trip: %v",
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
				4.90,
				19.80,
				'EUR',
				1.00,
				'payment-lifecycle-v1',
				$2
			)
		`,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create lifecycle fare: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM payments WHERE trip_id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup lifecycle payment: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup lifecycle trip: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup lifecycle ride request: %v",
				cleanupErr,
			)
		}
	}()

	service := NewService(
		Dependencies{
			DB: db,

			Payments: postgresrepo.NewPaymentRepository(db),
		},
	)

	created, err :=
		service.CreateForCompletedTrip(
			ctx,
			tripID,
			MethodCash,
		)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	if created.Status != StatusPending {
		t.Fatalf(
			"expected %s, got %s",
			StatusPending,
			created.Status,
		)
	}

	if created.Amount != "19.80" {
		t.Fatalf(
			"expected amount 19.80, got %s",
			created.Amount,
		)
	}

	if created.PaidAt != nil {
		t.Fatal(
			"expected paid_at to be nil before payment",
		)
	}

	// Hold the authoritative payment row lock.
	lockTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}

	lockCommitted := false

	defer func() {
		if !lockCommitted {
			_ = lockTx.Rollback(
				context.Background(),
			)
		}
	}()

	lockedPayments :=
		postgresrepo.NewPaymentRepositoryWithDB(
			lockTx,
		)

	_, err = lockedPayments.GetByIDForUpdate(
		ctx,
		created.ID,
	)
	if err != nil {
		t.Fatalf("lock payment row: %v", err)
	}

	type updateResult struct {
		err error
	}

	resultCh := make(
		chan updateResult,
		1,
	)

	go func() {
		_, updateErr :=
			service.UpdateStatus(
				context.Background(),
				created.ID,
				StatusPaid,
			)

		resultCh <- updateResult{
			err: updateErr,
		}
	}()

	// UpdateStatus must block while this transaction owns FOR UPDATE.
	select {
	case result := <-resultCh:
		t.Fatalf(
			"status update completed while payment lock was held: %v",
			result.err,
		)

	case <-time.After(250 * time.Millisecond):
		// Expected.
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf(
			"commit payment lock transaction: %v",
			err,
		)
	}

	lockCommitted = true

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf(
				"update payment to PAID: %v",
				result.err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"timed out waiting for blocked payment update",
		)
	}

	paid, err := service.GetByID(
		ctx,
		created.ID,
	)
	if err != nil {
		t.Fatalf("reload paid payment: %v", err)
	}

	if paid.Status != StatusPaid {
		t.Fatalf(
			"expected status %s, got %s",
			StatusPaid,
			paid.Status,
		)
	}

	if paid.PaidAt == nil {
		t.Fatal(
			"expected paid_at when payment becomes PAID",
		)
	}

	firstPaidAt := *paid.PaidAt

	// Same-state retry must be idempotent.
	sameState, err := service.UpdateStatus(
		ctx,
		created.ID,
		StatusPaid,
	)
	if err != nil {
		t.Fatalf(
			"idempotent PAID retry: %v",
			err,
		)
	}

	if sameState.PaidAt == nil {
		t.Fatal(
			"expected paid_at on idempotent PAID retry",
		)
	}

	if !sameState.PaidAt.Equal(firstPaidAt) {
		t.Fatalf(
			"paid_at changed on idempotent retry: first=%s second=%s",
			firstPaidAt,
			*sameState.PaidAt,
		)
	}

	// PAID cannot move backwards into processing.
	_, err = service.UpdateStatus(
		ctx,
		created.ID,
		StatusProcessing,
	)

	if !errors.Is(
		err,
		ErrInvalidPaymentTransition,
	) {
		t.Fatalf(
			"expected ErrInvalidPaymentTransition, got %v",
			err,
		)
	}

	finalPayment, err :=
		service.GetByID(
			ctx,
			created.ID,
		)
	if err != nil {
		t.Fatalf(
			"reload final payment: %v",
			err,
		)
	}

	if finalPayment.Status != StatusPaid {
		t.Fatalf(
			"expected final status %s, got %s",
			StatusPaid,
			finalPayment.Status,
		)
	}

	if finalPayment.PaidAt == nil ||
		!finalPayment.PaidAt.Equal(firstPaidAt) {

		t.Fatal(
			"expected authoritative paid_at to remain unchanged",
		)
	}
}
