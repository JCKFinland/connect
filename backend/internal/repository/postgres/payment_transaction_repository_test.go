package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestPaymentTransactionRepositoryRoundTripAndIdentityConstraints(
	t *testing.T,
) {
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
				'Payment Transaction Test Pickup',
				60.2055,
				24.6559,
				'Payment Transaction Test Destination',
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
			"create transaction test ride request: %v",
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
		t.Fatalf("create transaction test trip: %v", err)
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
				'payment-transaction-test-v1',
				$2
			)
		`,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf("create transaction test fare: %v", err)
	}

	paymentRepo := NewPaymentRepository(db)

	payment, err :=
		paymentRepo.CreateFromCompletedTrip(
			ctx,
			tripID,
			"CARD",
		)
	if err != nil {
		t.Fatalf("create transaction test payment: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM payments WHERE id = $1`,
			payment.ID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup transaction test payment: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup transaction test trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup transaction test ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewPaymentTransactionRepository(db)

	idempotencyKey := uuid.NewString()

	requestJSON := json.RawMessage(
		`{"source":"integration-test","capture":true}`,
	)

	transactionReference :=
		"txn_" + uuid.NewString()

	transaction, err := repo.Create(
		ctx,
		repository.CreatePaymentTransactionParams{
			PaymentID: payment.ID,

			TransactionReference: transactionReference,

			Provider: "TEST_PROVIDER",

			IdempotencyKey: &idempotencyKey,

			TransactionType: "SALE",

			Amount:   "23.45",
			Currency: "EUR",

			GatewayRequest: requestJSON,
		},
	)
	if err != nil {
		t.Fatalf(
			"create payment transaction: %v",
			err,
		)
	}

	if transaction.ID == "" {
		t.Fatal("expected transaction ID")
	}

	if transaction.PaymentID != payment.ID {
		t.Fatalf(
			"payment ID mismatch: got %s want %s",
			transaction.PaymentID,
			payment.ID,
		)
	}

	if transaction.TransactionReference !=
		transactionReference {

		t.Fatalf(
			"transaction reference mismatch: got %s want %s",
			transaction.TransactionReference,
			transactionReference,
		)
	}

	if transaction.Status != "PENDING" {
		t.Fatalf(
			"expected PENDING status, got %s",
			transaction.Status,
		)
	}

	if transaction.Amount != "23.45" {
		t.Fatalf(
			"expected exact amount 23.45, got %s",
			transaction.Amount,
		)
	}

	if transaction.Currency != "EUR" {
		t.Fatalf(
			"expected EUR currency, got %s",
			transaction.Currency,
		)
	}

	if transaction.IdempotencyKey == nil ||
		*transaction.IdempotencyKey != idempotencyKey {

		t.Fatal(
			"expected persisted idempotency key",
		)
	}

	var (
		gotGatewayRequest  map[string]any
		wantGatewayRequest map[string]any
	)

	if err := json.Unmarshal(
		transaction.GatewayRequest,
		&gotGatewayRequest,
	); err != nil {
		t.Fatalf(
			"decode persisted gateway request: %v",
			err,
		)
	}

	if err := json.Unmarshal(
		requestJSON,
		&wantGatewayRequest,
	); err != nil {
		t.Fatalf(
			"decode expected gateway request: %v",
			err,
		)
	}

	if len(gotGatewayRequest) != len(wantGatewayRequest) {
		t.Fatalf(
			"gateway request field count mismatch: got %v want %v",
			gotGatewayRequest,
			wantGatewayRequest,
		)
	}

	for key, wantValue := range wantGatewayRequest {
		gotValue, ok := gotGatewayRequest[key]
		if !ok {
			t.Fatalf(
				"gateway request missing key %q",
				key,
			)
		}

		if gotValue != wantValue {
			t.Fatalf(
				"gateway request value mismatch for %q: got %v want %v",
				key,
				gotValue,
				wantValue,
			)
		}
	}

	byReference, err :=
		repo.GetByReference(
			ctx,
			transactionReference,
		)
	if err != nil {
		t.Fatalf(
			"get transaction by reference: %v",
			err,
		)
	}

	if byReference.ID != transaction.ID {
		t.Fatalf(
			"reference lookup returned %s want %s",
			byReference.ID,
			transaction.ID,
		)
	}

	byIdempotency, err :=
		repo.GetByProviderIdempotencyKey(
			ctx,
			"TEST_PROVIDER",
			idempotencyKey,
		)
	if err != nil {
		t.Fatalf(
			"get transaction by idempotency key: %v",
			err,
		)
	}

	if byIdempotency.ID != transaction.ID {
		t.Fatalf(
			"idempotency lookup returned %s want %s",
			byIdempotency.ID,
			transaction.ID,
		)
	}

	// Same provider + same idempotency key must be rejected.
	_, err = repo.Create(
		ctx,
		repository.CreatePaymentTransactionParams{
			PaymentID: payment.ID,

			TransactionReference: "txn_" + uuid.NewString(),

			Provider: "TEST_PROVIDER",

			IdempotencyKey: &idempotencyKey,

			TransactionType: "SALE",

			Amount: "23.45",

			Currency: "EUR",
		},
	)

	if err == nil {
		t.Fatal(
			"expected duplicate provider idempotency key to fail",
		)
	}

	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) ||
		pgErr.Code != "23505" {

		t.Fatalf(
			"expected PostgreSQL unique violation for idempotency, got %v",
			err,
		)
	}

	// Assign an external provider transaction identifier to the
	// original row so we can prove migration 39 protects it.
	providerTransactionID :=
		"provider_" + uuid.NewString()

	_, err = db.Exec(
		ctx,
		`
			UPDATE payment_transactions
			SET
				provider_transaction_id = $1,
				updated_at = NOW()
			WHERE id = $2
		`,
		providerTransactionID,
		transaction.ID,
	)
	if err != nil {
		t.Fatalf(
			"assign provider transaction ID: %v",
			err,
		)
	}

	byProviderTransactionID, err :=
		repo.GetByProviderTransactionID(
			ctx,
			"TEST_PROVIDER",
			providerTransactionID,
		)
	if err != nil {
		t.Fatalf(
			"get by provider transaction ID: %v",
			err,
		)
	}

	if byProviderTransactionID.ID !=
		transaction.ID {

		t.Fatalf(
			"provider transaction lookup returned %s want %s",
			byProviderTransactionID.ID,
			transaction.ID,
		)
	}

	// The first transaction must leave the in-flight state before another
	// provider operation may be created for the same payment.
	if err := repo.UpdateResult(
		ctx,
		repository.UpdatePaymentTransactionResultParams{
			ID: transaction.ID,

			Status: "SUCCESS",

			ProviderTransactionID: &providerTransactionID,
		},
	); err != nil {
		t.Fatalf(
			"complete first transaction before second operation: %v",
			err,
		)
	}

	// Create another transaction with a different idempotency key,
	// then prove the same provider transaction ID cannot be reused.
	secondIdempotencyKey := uuid.NewString()

	second, err := repo.Create(
		ctx,
		repository.CreatePaymentTransactionParams{
			PaymentID: payment.ID,

			TransactionReference: "txn_" + uuid.NewString(),

			Provider: "TEST_PROVIDER",

			IdempotencyKey: &secondIdempotencyKey,

			TransactionType: "SALE",

			Amount: "23.45",

			Currency: "EUR",
		},
	)
	if err != nil {
		t.Fatalf(
			"create second transaction: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			UPDATE payment_transactions
			SET
				provider_transaction_id = $1,
				updated_at = NOW()
			WHERE id = $2
		`,
		providerTransactionID,
		second.ID,
	)

	if err == nil {
		t.Fatal(
			"expected duplicate provider transaction ID to fail",
		)
	}

	pgErr = nil

	if !errors.As(err, &pgErr) ||
		pgErr.Code != "23505" {

		t.Fatalf(
			"expected PostgreSQL unique violation for provider transaction ID, got %v",
			err,
		)
	}
}
