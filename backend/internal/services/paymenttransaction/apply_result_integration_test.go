package paymenttransaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestApplyResultIsSerializedIdempotentAndPreservesProviderIdentity(
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

	// Build a self-contained completed-trip, fare, and payment fixture.
	// The test verifies transactional reconciliation between the provider
	// transaction and the authoritative aggregate payment.
	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

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
				'Transaction Reconciliation Test Pickup',
				60.2055,
				24.6559,
				'Transaction Reconciliation Test Destination',
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
			"create reconciliation ride request: %v",
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
			"create reconciliation trip: %v",
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
				1.00,
				'EUR',
				1.00,
				'reconciliation-test-v1',
				$2
			)
		`,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create reconciliation fare: %v",
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
		t.Fatalf(
			"create reconciliation payment: %v",
			err,
		)
	}

	paymentID := payment.ID

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM payments WHERE id = $1`,
			paymentID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup reconciliation payment: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup reconciliation trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup reconciliation ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo :=
		postgresrepo.NewPaymentTransactionRepository(db)

	idempotencyKey := uuid.NewString()

	transaction, err := repo.Create(
		ctx,
		repository.CreatePaymentTransactionParams{
			PaymentID: paymentID,

			TransactionReference: "txn_" + uuid.NewString(),

			Provider: "TEST_PROVIDER",

			IdempotencyKey: &idempotencyKey,

			TransactionType: "SALE",

			Amount: "1.00",

			Currency: "EUR",

			GatewayRequest: json.RawMessage(`{"test":true}`),
		},
	)
	if err != nil {
		t.Fatalf(
			"create reconciliation transaction: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM payment_transactions
				WHERE id = $1
			`,
			transaction.ID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup reconciliation transaction: %v",
				cleanupErr,
			)
		}
	}()

	service := NewService(db)

	// 1. Prove ApplyResult serializes all financial operations
	//    through the authoritative parent payment row.

	lockTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf(
			"begin lock transaction: %v",
			err,
		)
	}

	lockedPayments :=
		postgresrepo.NewPaymentRepositoryWithDB(
			lockTx,
		)

	_, err = lockedPayments.GetByIDForUpdate(
		ctx,
		paymentID,
	)
	if err != nil {
		_ = lockTx.Rollback(ctx)

		t.Fatalf(
			"lock aggregate payment row: %v",
			err,
		)
	}

	type result struct {
		transactionID string
		err           error
	}

	resultCh := make(
		chan result,
		1,
	)

	go func() {
		updated, updateErr :=
			service.ApplyResult(
				context.Background(),
				transaction.ID,
				ApplyResultRequest{
					Status: StatusProcessing,
				},
			)

		var id string

		if updated != nil {
			id = updated.ID
		}

		resultCh <- result{
			transactionID: id,
			err:           updateErr,
		}
	}()

	select {
	case result := <-resultCh:
		_ = lockTx.Rollback(ctx)

		t.Fatalf(
			"ApplyResult completed while row lock was held: %+v",
			result,
		)

	case <-time.After(250 * time.Millisecond):
		// Expected: waiting on SELECT ... FOR UPDATE.
	}

	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf(
			"commit lock transaction: %v",
			err,
		)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf(
				"apply PROCESSING result: %v",
				result.err,
			)
		}

		if result.transactionID != transaction.ID {
			t.Fatalf(
				"unexpected transaction ID: got %s want %s",
				result.transactionID,
				transaction.ID,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"timed out waiting for serialized ApplyResult",
		)
	}

	// ---------------------------------------------------------
	// 2. Terminal provider result sets processed_at.
	// ---------------------------------------------------------

	successResponse :=
		json.RawMessage(`{"status":"success"}`)

	success, err :=
		service.ApplyResult(
			ctx,
			transaction.ID,
			ApplyResultRequest{
				Status: StatusSuccess,

				GatewayResponse: successResponse,
			},
		)
	if err != nil {
		t.Fatalf(
			"apply SUCCESS result: %v",
			err,
		)
	}

	aggregatePayment, err :=
		postgresrepo.NewPaymentRepository(db).
			GetByID(
				ctx,
				paymentID,
			)
	if err != nil {
		t.Fatalf(
			"reload aggregate payment after SALE success: %v",
			err,
		)
	}

	if aggregatePayment.Status != "PAID" {
		t.Fatalf(
			"expected aggregate payment PAID after SALE success, got %s",
			aggregatePayment.Status,
		)
	}

	if aggregatePayment.PaidAt == nil {
		t.Fatal(
			"expected aggregate payment paid_at after SALE success",
		)
	}

	if success.Status != StatusSuccess {
		t.Fatalf(
			"expected SUCCESS, got %s",
			success.Status,
		)
	}

	if success.ProcessedAt == nil {
		t.Fatal(
			"expected processed_at for terminal transaction",
		)
	}

	firstProcessedAt := *success.ProcessedAt

	// ---------------------------------------------------------
	// 3. Same-state delivery may fill missing provider ID once.
	// ---------------------------------------------------------

	providerTransactionID :=
		"provider_" + uuid.NewString()

	enriched, err :=
		service.ApplyResult(
			ctx,
			transaction.ID,
			ApplyResultRequest{
				Status: StatusSuccess,

				ProviderTransactionID: &providerTransactionID,
			},
		)
	if err != nil {
		t.Fatalf(
			"enrich provider transaction identity: %v",
			err,
		)
	}

	if enriched.ProviderTransactionID == nil ||
		*enriched.ProviderTransactionID !=
			providerTransactionID {

		t.Fatal(
			"expected provider transaction ID enrichment",
		)
	}

	if enriched.ProcessedAt == nil ||
		!enriched.ProcessedAt.Equal(
			firstProcessedAt,
		) {

		t.Fatal(
			"processed_at changed during idempotent enrichment",
		)
	}

	// ---------------------------------------------------------
	// 4. Exact retry is idempotent.
	// ---------------------------------------------------------

	retry, err :=
		service.ApplyResult(
			ctx,
			transaction.ID,
			ApplyResultRequest{
				Status: StatusSuccess,

				ProviderTransactionID: &providerTransactionID,
			},
		)
	if err != nil {
		t.Fatalf(
			"idempotent provider retry: %v",
			err,
		)
	}

	if retry.ProcessedAt == nil ||
		!retry.ProcessedAt.Equal(
			firstProcessedAt,
		) {

		t.Fatal(
			"processed_at changed on duplicate provider delivery",
		)
	}

	// ---------------------------------------------------------
	// 5. Provider identity cannot be rewritten.
	// ---------------------------------------------------------

	conflictingProviderID :=
		"provider_" + uuid.NewString()

	_, err = service.ApplyResult(
		ctx,
		transaction.ID,
		ApplyResultRequest{
			Status: StatusSuccess,

			ProviderTransactionID: &conflictingProviderID,
		},
	)

	if !errors.Is(
		err,
		ErrProviderIdentityConflict,
	) {
		t.Fatalf(
			"expected ErrProviderIdentityConflict, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 6. Terminal state cannot transition elsewhere.
	// ---------------------------------------------------------

	_, err = service.ApplyResult(
		ctx,
		transaction.ID,
		ApplyResultRequest{
			Status: StatusFailed,
		},
	)

	if !errors.Is(
		err,
		ErrInvalidTransactionTransition,
	) {
		t.Fatalf(
			"expected ErrInvalidTransactionTransition, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. AUTHORIZE SUCCESS -> aggregate payment AUTHORIZED.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
		UPDATE payments
		SET
			status = 'PENDING',
			paid_at = NULL,
			updated_at = NOW()
		WHERE id = $1
	`,
		paymentID,
	)
	if err != nil {
		t.Fatalf(
			"reset payment before AUTHORIZE test: %v",
			err,
		)
	}

	authorizeTx := createTestPaymentTransaction(
		t,
		ctx,
		repo,
		paymentID,
		TypeAuthorize,
		"1.00",
	)

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM payment_transactions WHERE id = $1`,
			authorizeTx.ID,
		)
	}()

	_, err = service.ApplyResult(
		ctx,
		authorizeTx.ID,
		ApplyResultRequest{
			Status: StatusSuccess,
		},
	)
	if err != nil {
		t.Fatalf(
			"apply AUTHORIZE success: %v",
			err,
		)
	}

	authorizedPayment, err :=
		postgresrepo.NewPaymentRepository(db).
			GetByID(
				ctx,
				paymentID,
			)
	if err != nil {
		t.Fatalf(
			"reload payment after AUTHORIZE: %v",
			err,
		)
	}

	if authorizedPayment.Status != "AUTHORIZED" {
		t.Fatalf(
			"expected AUTHORIZED after AUTHORIZE success, got %s",
			authorizedPayment.Status,
		)
	}

	if authorizedPayment.PaidAt != nil {
		t.Fatal(
			"expected paid_at to remain nil after authorization only",
		)
	}
	// ---------------------------------------------------------
	// 8. Cumulative REFUND SUCCESS -> PARTIAL -> REFUNDED.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
		UPDATE payments
		SET
			status = 'PAID',
			paid_at = COALESCE(paid_at, NOW()),
			updated_at = NOW()
		WHERE id = $1
	`,
		paymentID,
	)
	if err != nil {
		t.Fatalf(
			"prepare payment for refund test: %v",
			err,
		)
	}

	refundOne := createTestPaymentTransaction(
		t,
		ctx,
		repo,
		paymentID,
		TypeRefund,
		"0.40",
	)

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM payment_transactions WHERE id = $1`,
			refundOne.ID,
		)
	}()

	_, err = service.ApplyResult(
		ctx,
		refundOne.ID,
		ApplyResultRequest{
			Status: StatusSuccess,
		},
	)
	if err != nil {
		t.Fatalf(
			"apply first refund success: %v",
			err,
		)
	}

	partialPayment, err :=
		postgresrepo.NewPaymentRepository(db).
			GetByID(
				ctx,
				paymentID,
			)
	if err != nil {
		t.Fatalf(
			"reload payment after partial refund: %v",
			err,
		)
	}

	if partialPayment.Status != "PARTIALLY_REFUNDED" {
		t.Fatalf(
			"expected PARTIALLY_REFUNDED, got %s",
			partialPayment.Status,
		)
	}

	refundTwo := createTestPaymentTransaction(
		t,
		ctx,
		repo,
		paymentID,
		TypeRefund,
		"0.60",
	)

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM payment_transactions WHERE id = $1`,
			refundTwo.ID,
		)
	}()

	_, err = service.ApplyResult(
		ctx,
		refundTwo.ID,
		ApplyResultRequest{
			Status: StatusSuccess,
		},
	)
	if err != nil {
		t.Fatalf(
			"apply second refund success: %v",
			err,
		)
	}

	refundedPayment, err :=
		postgresrepo.NewPaymentRepository(db).
			GetByID(
				ctx,
				paymentID,
			)
	if err != nil {
		t.Fatalf(
			"reload fully refunded payment: %v",
			err,
		)
	}

	if refundedPayment.Status != "REFUNDED" {
		t.Fatalf(
			"expected REFUNDED after cumulative full refund, got %s",
			refundedPayment.Status,
		)
	}
	// ---------------------------------------------------------
	// 9. Over-refund rolls back transaction SUCCESS atomically.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
		UPDATE payments
		SET
			status = 'PAID',
			updated_at = NOW()
		WHERE id = $1
	`,
		paymentID,
	)
	if err != nil {
		t.Fatalf(
			"reset payment before over-refund test: %v",
			err,
		)
	}

	// Remove earlier refund evidence so this test starts from zero.
	_, err = db.Exec(
		ctx,
		`
		DELETE FROM payment_transactions
		WHERE payment_id = $1
		  AND transaction_type = 'REFUND'
	`,
		paymentID,
	)
	if err != nil {
		t.Fatalf(
			"clear refund transactions before over-refund test: %v",
			err,
		)
	}

	overRefund := createTestPaymentTransaction(
		t,
		ctx,
		repo,
		paymentID,
		TypeRefund,
		"1.01",
	)

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM payment_transactions WHERE id = $1`,
			overRefund.ID,
		)
	}()

	_, err = service.ApplyResult(
		ctx,
		overRefund.ID,
		ApplyResultRequest{
			Status: StatusSuccess,
		},
	)

	if !errors.Is(
		err,
		ErrRefundExceedsPaymentAmount,
	) {
		t.Fatalf(
			"expected ErrRefundExceedsPaymentAmount, got %v",
			err,
		)
	}

	rolledBackTransaction, err :=
		repo.GetByID(
			ctx,
			overRefund.ID,
		)
	if err != nil {
		t.Fatalf(
			"reload over-refund transaction: %v",
			err,
		)
	}

	if rolledBackTransaction.Status != StatusPending {
		t.Fatalf(
			"expected over-refund transaction rollback to PENDING, got %s",
			rolledBackTransaction.Status,
		)
	}

	rolledBackPayment, err :=
		postgresrepo.NewPaymentRepository(db).
			GetByID(
				ctx,
				paymentID,
			)
	if err != nil {
		t.Fatalf(
			"reload payment after over-refund rollback: %v",
			err,
		)
	}

	if rolledBackPayment.Status != "PAID" {
		t.Fatalf(
			"expected payment to remain PAID after over-refund rollback, got %s",
			rolledBackPayment.Status,
		)
	}

}

func createTestPaymentTransaction(
	t *testing.T,
	ctx context.Context,
	repo *postgresrepo.PaymentTransactionRepository,
	paymentID string,
	transactionType string,
	amount string,
) *models.PaymentTransaction {
	t.Helper()

	idempotencyKey := uuid.NewString()

	transaction, err := repo.Create(
		ctx,
		repository.CreatePaymentTransactionParams{
			PaymentID: paymentID,

			TransactionReference: "txn_" + uuid.NewString(),

			Provider: "TEST_PROVIDER",

			IdempotencyKey: &idempotencyKey,

			TransactionType: transactionType,

			Amount: amount,

			Currency: "EUR",

			GatewayRequest: json.RawMessage(`{"test":true}`),
		},
	)
	if err != nil {
		t.Fatalf(
			"create %s payment transaction: %v",
			transactionType,
			err,
		)
	}

	return transaction
}
