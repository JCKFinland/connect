package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

type paymentTransactionParentFixture struct {
	Payment *models.Payment

	TripID        string
	RideRequestID string
}

func TestPaymentTransactionRepositorySuccessfulParentLookup(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)
	if err != nil {
		t.Fatalf(
			"acquire fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release fixture lock: %v",
				err,
			)
		}
	}()

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create isolated driver fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupDriverFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup isolated driver fixture: %v",
				err,
			)
		}
	}()

	fixture :=
		createPaymentTransactionParentFixture(
			t,
			ctx,
			db,
			driverFixture,
		)

	defer func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM payments WHERE id = $1`,
			fixture.Payment.ID,
		); err != nil {
			t.Logf(
				"cleanup parent lookup payment: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM trips WHERE id = $1`,
			fixture.TripID,
		); err != nil {
			t.Logf(
				"cleanup parent lookup trip: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			fixture.RideRequestID,
		); err != nil {
			t.Logf(
				"cleanup parent lookup ride request: %v",
				err,
			)
		}
	}()

	repo :=
		NewPaymentTransactionRepository(db)

	// -------------------------------------------------------------
	// No successful eligible parent exists initially.
	// -------------------------------------------------------------

	_, err =
		repo.GetLatestSuccessfulByPaymentAndTypes(
			ctx,
			fixture.Payment.ID,
			"TEST_PROVIDER",
			[]string{
				"SALE",
				"CAPTURE",
			},
		)

	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected repository.ErrNotFound before successful parent exists, got %v",
			err,
		)
	}

	// -------------------------------------------------------------
	// Successful SALE becomes the initial REFUND parent.
	// -------------------------------------------------------------

	sale :=
		createParentLookupTransaction(
			t,
			ctx,
			repo,
			fixture.Payment.ID,
			"SALE",
		)

	saleProviderID :=
		"pi_sale_" + uuid.NewString()

	completeParentLookupTransaction(
		t,
		ctx,
		repo,
		sale.ID,
		"SUCCESS",
		&saleProviderID,
	)

	refundParent, err :=
		repo.GetLatestSuccessfulByPaymentAndTypes(
			ctx,
			fixture.Payment.ID,
			"TEST_PROVIDER",
			[]string{
				"SALE",
				"CAPTURE",
			},
		)
	if err != nil {
		t.Fatalf(
			"resolve successful SALE refund parent: %v",
			err,
		)
	}

	assertPaymentTransactionParent(
		t,
		refundParent,
		sale.ID,
		saleProviderID,
	)

	// -------------------------------------------------------------
	// FAILED CAPTURE must not replace successful SALE.
	// -------------------------------------------------------------

	failedCapture :=
		createParentLookupTransaction(
			t,
			ctx,
			repo,
			fixture.Payment.ID,
			"CAPTURE",
		)

	failedCaptureProviderID :=
		"pi_failed_capture_" + uuid.NewString()

	completeParentLookupTransaction(
		t,
		ctx,
		repo,
		failedCapture.ID,
		"FAILED",
		&failedCaptureProviderID,
	)

	refundParent, err =
		repo.GetLatestSuccessfulByPaymentAndTypes(
			ctx,
			fixture.Payment.ID,
			"TEST_PROVIDER",
			[]string{
				"SALE",
				"CAPTURE",
			},
		)
	if err != nil {
		t.Fatalf(
			"resolve refund parent after failed CAPTURE: %v",
			err,
		)
	}

	assertPaymentTransactionParent(
		t,
		refundParent,
		sale.ID,
		saleProviderID,
	)

	// -------------------------------------------------------------
	// Newer successful CAPTURE supersedes SALE for REFUND.
	// -------------------------------------------------------------

	capture :=
		createParentLookupTransaction(
			t,
			ctx,
			repo,
			fixture.Payment.ID,
			"CAPTURE",
		)

	captureProviderID :=
		"pi_capture_" + uuid.NewString()

	completeParentLookupTransaction(
		t,
		ctx,
		repo,
		capture.ID,
		"SUCCESS",
		&captureProviderID,
	)

	refundParent, err =
		repo.GetLatestSuccessfulByPaymentAndTypes(
			ctx,
			fixture.Payment.ID,
			"TEST_PROVIDER",
			[]string{
				"SALE",
				"CAPTURE",
			},
		)
	if err != nil {
		t.Fatalf(
			"resolve latest successful refund parent: %v",
			err,
		)
	}

	assertPaymentTransactionParent(
		t,
		refundParent,
		capture.ID,
		captureProviderID,
	)

	// -------------------------------------------------------------
	// Successful AUTHORIZE becomes CAPTURE / VOID parent.
	// -------------------------------------------------------------

	authorize :=
		createParentLookupTransaction(
			t,
			ctx,
			repo,
			fixture.Payment.ID,
			"AUTHORIZE",
		)

	authorizeProviderID :=
		"pi_authorize_" + uuid.NewString()

	completeParentLookupTransaction(
		t,
		ctx,
		repo,
		authorize.ID,
		"SUCCESS",
		&authorizeProviderID,
	)

	authorizationParent, err :=
		repo.GetLatestSuccessfulByPaymentAndTypes(
			ctx,
			fixture.Payment.ID,
			"TEST_PROVIDER",
			[]string{
				"AUTHORIZE",
			},
		)
	if err != nil {
		t.Fatalf(
			"resolve successful AUTHORIZE parent: %v",
			err,
		)
	}

	assertPaymentTransactionParent(
		t,
		authorizationParent,
		authorize.ID,
		authorizeProviderID,
	)

	// -------------------------------------------------------------
	// Provider isolation.
	// -------------------------------------------------------------

	_, err =
		repo.GetLatestSuccessfulByPaymentAndTypes(
			ctx,
			fixture.Payment.ID,
			"OTHER_PROVIDER",
			[]string{
				"SALE",
				"CAPTURE",
				"AUTHORIZE",
			},
		)

	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected provider-isolated lookup to return repository.ErrNotFound, got %v",
			err,
		)
	}
}

func createPaymentTransactionParentFixture(
	t *testing.T,
	ctx context.Context,
	db DBTX,
	driverFixture *testutil.DriverFixture,
) *paymentTransactionParentFixture {
	t.Helper()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	driverID := driverFixture.UserID
	companyID := driverFixture.CompanyID
	branchID := driverFixture.BranchID
	fleetID := driverFixture.FleetID
	vehicleID := driverFixture.VehicleID

	now := time.Now().UTC()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				'Parent Lookup Test Pickup',
				60.2055,
				24.6559,
				'Parent Lookup Test Destination',
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
			"create parent lookup ride request: %v",
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
		t.Fatalf(
			"create parent lookup trip: %v",
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
				23.45,
				'EUR',
				1.00,
				'parent-lookup-test-v1',
				$2
			)
		`,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create parent lookup fare: %v",
			err,
		)
	}

	paymentRepo :=
		NewPaymentRepositoryWithDB(db)

	payment, err :=
		paymentRepo.CreateFromCompletedTrip(
			ctx,
			tripID,
			"CARD",
		)
	if err != nil {
		t.Fatalf(
			"create parent lookup payment: %v",
			err,
		)
	}

	return &paymentTransactionParentFixture{
		Payment: payment,

		TripID: tripID,

		RideRequestID: rideRequestID,
	}
}

func createParentLookupTransaction(
	t *testing.T,
	ctx context.Context,
	repo *PaymentTransactionRepository,
	paymentID string,
	transactionType string,
) *models.PaymentTransaction {
	t.Helper()

	idempotencyKey :=
		uuid.NewString()

	transaction, err :=
		repo.Create(
			ctx,
			repository.CreatePaymentTransactionParams{
				PaymentID: paymentID,

				TransactionReference: "txn_" + uuid.NewString(),

				Provider: "TEST_PROVIDER",

				IdempotencyKey: &idempotencyKey,

				TransactionType: transactionType,

				Amount: "23.45",

				Currency: "EUR",
			},
		)
	if err != nil {
		t.Fatalf(
			"create %s parent lookup transaction: %v",
			transactionType,
			err,
		)
	}

	return transaction
}

func completeParentLookupTransaction(
	t *testing.T,
	ctx context.Context,
	repo *PaymentTransactionRepository,
	transactionID string,
	status string,
	providerTransactionID *string,
) {
	t.Helper()

	if err := repo.UpdateResult(
		ctx,
		repository.UpdatePaymentTransactionResultParams{
			ID: transactionID,

			Status: status,

			ProviderTransactionID: providerTransactionID,
		},
	); err != nil {
		t.Fatalf(
			"update parent lookup transaction %s to %s: %v",
			transactionID,
			status,
			err,
		)
	}
}

func assertPaymentTransactionParent(
	t *testing.T,
	got *models.PaymentTransaction,
	wantID string,
	wantProviderTransactionID string,
) {
	t.Helper()

	if got == nil {
		t.Fatal(
			"expected payment transaction parent",
		)
	}

	if got.ID != wantID {
		t.Fatalf(
			"parent transaction ID mismatch: got %s want %s",
			got.ID,
			wantID,
		)
	}

	if got.ProviderTransactionID == nil {
		t.Fatal(
			"expected parent provider transaction ID",
		)
	}

	if *got.ProviderTransactionID !=
		wantProviderTransactionID {

		t.Fatalf(
			"parent provider transaction ID mismatch: got %s want %s",
			*got.ProviderTransactionID,
			wantProviderTransactionID,
		)
	}
}
