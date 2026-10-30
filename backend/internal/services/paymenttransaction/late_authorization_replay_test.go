package paymenttransaction

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
	paymentservice "github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestLateAuthorizationSuccessCannotDowngradePaidPayment(t *testing.T) {
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
		t.Fatalf("load configuration: %v", err)
	}

	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatalf("unsafe integration database: %s", os.Getenv("DB_NAME"))
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	defer db.Close()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup customer: %v", err)
		}
	}()

	driver, cleanupDriver, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	defer func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Logf("cleanup driver: %v", err)
		}
	}()

	now := time.Now().UTC()
	rideID := uuid.NewString()
	tripID := uuid.NewString()

	_, err = db.Exec(ctx, `
        INSERT INTO ride_requests (
            id, customer_id,
            pickup_address, pickup_latitude, pickup_longitude,
            destination_address, destination_latitude,
            destination_longitude, requested_vehicle_type,
            passenger_count, status, requested_at, expires_at,
            created_at, updated_at
        )
        VALUES (
            $1, $2,
            'Late Authorization Pickup', 60.2055, 24.6559,
            'Late Authorization Destination', 60.1719, 24.9414,
            'STANDARD', 1, 'ACCEPTED', $3, $4, $3, $3
        )
    `, rideID, customerID, now, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
	}()

	_, err = db.Exec(ctx, `
        INSERT INTO trips (
            id, ride_request_id, customer_id, driver_id,
            vehicle_id, company_id, branch_id, fleet_id,
            status, assigned_at, driver_arrived_at, started_at,
            completed_at, is_active, created_at, updated_at
        )
        VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8,
            'COMPLETED', $9, $10, $10, $11,
            FALSE, $9, $11
        )
    `,
		tripID, rideID, customerID, driver.UserID,
		driver.VehicleID, driver.CompanyID, driver.BranchID,
		driver.FleetID, now.Add(-20*time.Minute),
		now.Add(-10*time.Minute), now,
	)
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		)
	}()

	_, err = db.Exec(ctx, `
        INSERT INTO trip_fares (
            trip_id, base_fare, total_amount, currency,
            surge_multiplier, pricing_version, calculated_at
        )
        VALUES ($1, 4.90, 1.00, 'EUR', 1.00, 'late-auth-test-v1', $2)
    `, tripID, now)
	if err != nil {
		t.Fatalf("create fare: %v", err)
	}

	paymentRepo := postgresrepo.NewPaymentRepository(db)
	payment, err := paymentRepo.CreateFromCompletedTrip(
		ctx, tripID, "CARD",
	)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM payments WHERE id = $1`,
			payment.ID,
		)
	}()

	transactionRepo :=
		postgresrepo.NewPaymentTransactionRepository(db)

	authorization := createTestPaymentTransaction(
		t, ctx, transactionRepo,
		payment.ID, TypeAuthorize, "1.00",
	)

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM payment_transactions WHERE id = $1`,
			authorization.ID,
		)
	}()

	_, err = db.Exec(ctx, `
        UPDATE payments
        SET status = 'PAID',
            paid_at = NOW(),
            updated_at = NOW()
        WHERE id = $1
    `, payment.ID)
	if err != nil {
		t.Fatalf("prepare paid payment: %v", err)
	}

	before, err := paymentRepo.GetByID(ctx, payment.ID)
	if err != nil {
		t.Fatalf("read payment before replay: %v", err)
	}

	if before.Status != paymentservice.StatusPaid ||
		before.PaidAt == nil {
		t.Fatalf("expected paid payment before replay: %+v", before)
	}

	service := NewService(db)

	_, err = service.ApplyResult(
		ctx,
		authorization.ID,
		ApplyResultRequest{
			Status: StatusSuccess,
		},
	)

	if !errors.Is(
		err,
		paymentservice.ErrInvalidPaymentTransition,
	) {
		t.Fatalf(
			"expected rejected PAID to AUTHORIZED transition, got %v",
			err,
		)
	}

	after, err := paymentRepo.GetByID(ctx, payment.ID)
	if err != nil {
		t.Fatalf("reload payment: %v", err)
	}

	if after.Status != paymentservice.StatusPaid {
		t.Fatalf(
			"late authorization downgraded payment to %s",
			after.Status,
		)
	}

	if after.PaidAt == nil ||
		!after.PaidAt.Equal(*before.PaidAt) {
		t.Fatal("late authorization changed payment paid_at")
	}

	transaction, err := transactionRepo.GetByID(
		ctx, authorization.ID,
	)
	if err != nil {
		t.Fatalf("reload authorization transaction: %v", err)
	}

	if transaction.Status != StatusPending {
		t.Fatalf(
			"late authorization transaction did not roll back: %s",
			transaction.Status,
		)
	}

	if transaction.ProcessedAt != nil {
		t.Fatal("rejected authorization set processed_at")
	}
}
