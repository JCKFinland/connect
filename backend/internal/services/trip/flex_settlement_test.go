package trip

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"

	"strings"
	"time"
)

func TestFlexSettlementRequiresCompletedAuthorizedTrip(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	f := testutil.SetupFlexAcceptFixture(t)

	testutil.RegisterFlexTripCleanup(
		t,
		f.DB,
		f.RideID,
		f.AuctionID,
	)

	ctx := context.Background()

	// A fixture containing only an open auction cannot produce
	// a financially valid settlement.
	settlementID := uuid.NewString()
	captureKey := "flex-capture-" + uuid.NewString()

	_, err := f.DB.Exec(ctx, `
        INSERT INTO flex_settlements (
            id,
            trip_id,
            authorization_id,
            amount,
            currency,
            provider,
            provider_authorization_id,
            capture_idempotency_key,
            status
        )
        VALUES (
            $1,
            $2,
            $3,
            23.28,
            'EUR',
            'STRIPE',
            $4,
            $5,
            'PENDING'
        )
    `,
		settlementID,
		uuid.NewString(),
		uuid.NewString(),
		"pi_invalid_"+uuid.NewString(),
		captureKey,
	)

	if err == nil {
		t.Fatal(
			"settlement unexpectedly accepted without a completed " +
				"authorized FLEX trip",
		)
	}

	var count int

	if err := f.DB.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_settlements
        WHERE id = $1
    `, settlementID).Scan(&count); err != nil {
		t.Fatalf("count rejected FLEX settlement: %v", err)
	}

	if count != 0 {
		t.Fatalf(
			"invalid FLEX settlement persisted: count=%d",
			count,
		)
	}
}

func TestFlexSettlementPendingCreation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify settlement database: %v", err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing settlement integration test on database %q",
			databaseName,
		)
	}

	// CompleteTrip must already have committed the PENDING settlement.
	// Read its authoritative database-generated identity rather than
	// inserting a second settlement for the same completed trip.
	var persistedID string
	var persistedTripID string
	var persistedAuthorizationID string
	var persistedAmount string
	var persistedCurrency string
	var persistedProvider string
	var persistedProviderID string
	var persistedCaptureKey string
	var persistedStatus string

	err := db.QueryRow(ctx, `
		SELECT
			id,
			trip_id,
			authorization_id,
			amount::text,
			currency,
			provider,
			provider_authorization_id,
			capture_idempotency_key,
			status
		FROM flex_settlements
		WHERE trip_id = $1
	`, lifecycle.TripID).Scan(
		&persistedID,
		&persistedTripID,
		&persistedAuthorizationID,
		&persistedAmount,
		&persistedCurrency,
		&persistedProvider,
		&persistedProviderID,
		&persistedCaptureKey,
		&persistedStatus,
	)
	if err != nil {
		t.Fatalf("read automatically created FLEX settlement: %v", err)
	}

	expectedAmount := fmt.Sprintf(
		"%.2f",
		float64(lifecycle.AmountCents)/100,
	)

	if persistedID == "" ||
		persistedTripID != lifecycle.TripID ||
		persistedAuthorizationID != lifecycle.AuthorizationID ||
		persistedAmount != expectedAmount ||
		persistedCurrency != lifecycle.Currency ||
		persistedProvider != "STRIPE" ||
		persistedProviderID != lifecycle.ProviderID ||
		persistedCaptureKey != "flex-capture:"+lifecycle.TripID ||
		persistedStatus != "PENDING" {
		t.Fatalf(
			"persisted settlement identity mismatch: "+
				"id=%s trip=%s authorization=%s amount=%s "+
				"currency=%s provider=%s providerID=%s "+
				"captureKey=%s status=%s",
			persistedID,
			persistedTripID,
			persistedAuthorizationID,
			persistedAmount,
			persistedCurrency,
			persistedProvider,
			persistedProviderID,
			persistedCaptureKey,
			persistedStatus,
		)
	}

	// A completed FLEX trip must never have two settlements.
	_, err = db.Exec(ctx, `
        INSERT INTO flex_settlements (
            trip_id,
            authorization_id,
            amount,
            currency,
            provider,
            provider_authorization_id,
            capture_idempotency_key,
            status
        )
        VALUES (
            $1,
            $2,
            ($3::bigint::numeric / 100),
            $4,
            'STRIPE',
            $5,
            $6,
            'PENDING'
        )
    `,
		lifecycle.TripID,
		lifecycle.AuthorizationID,
		lifecycle.AmountCents,
		lifecycle.Currency,
		lifecycle.ProviderID,
		"flex-duplicate-"+uuid.NewString(),
	)

	if err == nil {
		t.Fatal("duplicate FLEX settlement unexpectedly accepted")
	}

	var count int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_settlements
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&count); err != nil {
		t.Fatalf("count FLEX settlements: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected exactly one settlement, got %d",
			count,
		)
	}
}

// TestFlexSettlementRepositoryConcurrentCreation verifies that repeated
// and concurrent settlement requests cannot create multiple financial
// settlement identities for the same completed FLEX trip.
func TestFlexSettlementRepositoryConcurrentCreation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify settlement database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing repository test on database %q",
			databaseName,
		)
	}

	const workers = 8

	type result struct {
		settlement *postgres.FlexSettlement
		err        error
	}

	results := make([]result, workers)

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			<-start

			settlement, err :=
				postgres.CreateFlexSettlementFromCompletedTrip(
					ctx,
					db,
					lifecycle.TripID,
				)

			results[index] = result{
				settlement: settlement,
				err:        err,
			}
		}(i)
	}

	close(start)
	wg.Wait()

	var settlementID string

	for index, outcome := range results {
		if outcome.err != nil {
			t.Fatalf(
				"concurrent settlement request %d: %v",
				index+1,
				outcome.err,
			)
		}

		settlement := outcome.settlement

		if settlement == nil {
			t.Fatalf(
				"concurrent settlement request %d returned nil",
				index+1,
			)
		}

		if settlementID == "" {
			settlementID = settlement.ID
		}

		if settlement.ID != settlementID {
			t.Fatalf(
				"concurrent requests returned different settlements: %s != %s",
				settlement.ID,
				settlementID,
			)
		}

		if settlement.TripID != lifecycle.TripID ||
			settlement.AuthorizationID != lifecycle.AuthorizationID ||
			settlement.ProviderAuthorizationID != lifecycle.ProviderID ||
			settlement.AmountCents != lifecycle.AmountCents ||
			settlement.Currency != lifecycle.Currency ||
			settlement.Provider != "STRIPE" ||
			settlement.Status != "PENDING" ||
			settlement.AttemptCount != 0 ||
			settlement.CaptureIdempotencyKey !=
				"flex-capture:"+lifecycle.TripID {
			t.Fatalf(
				"concurrent request %d returned invalid settlement: %+v",
				index+1,
				settlement,
			)
		}
	}

	// A subsequent sequential call must also return the same identity.
	repeated, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)

	if err != nil {
		t.Fatalf("repeat settlement creation: %v", err)
	}

	if repeated == nil || repeated.ID != settlementID {
		t.Fatalf(
			"repeated creation returned different settlement: %+v",
			repeated,
		)
	}

	var count int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_settlements
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&count); err != nil {
		t.Fatalf("count persisted FLEX settlements: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected exactly one persisted settlement, got %d",
			count,
		)
	}
}

// TestFlexSettlementConcurrentProcessingClaim verifies that PostgreSQL
// permits only one worker to claim a PENDING FLEX settlement.
func TestFlexSettlementConcurrentProcessingClaim(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify settlement database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing claim test on database %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)

	if err != nil {
		t.Fatalf("create settlement before claiming: %v", err)
	}

	if settlement == nil || settlement.Status != "PENDING" {
		t.Fatalf(
			"expected PENDING settlement, got %+v",
			settlement,
		)
	}

	const workers = 8

	type claimResult struct {
		settlement *postgres.FlexSettlement
		claimed    bool
		err        error
	}

	results := make([]claimResult, workers)

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			<-start

			current, claimed, err :=
				postgres.ClaimFlexSettlementProcessing(
					ctx,
					db,
					settlement.ID,
				)

			results[index] = claimResult{
				settlement: current,
				claimed:    claimed,
				err:        err,
			}
		}(i)
	}

	close(start)
	wg.Wait()

	successfulClaims := 0

	for index, outcome := range results {
		if outcome.err != nil {
			t.Fatalf(
				"claim worker %d failed: %v",
				index+1,
				outcome.err,
			)
		}

		if outcome.settlement == nil {
			t.Fatalf(
				"claim worker %d returned nil settlement",
				index+1,
			)
		}

		if outcome.settlement.ID != settlement.ID {
			t.Fatalf(
				"claim worker %d returned different settlement",
				index+1,
			)
		}

		if outcome.settlement.Status != "PROCESSING" {
			t.Fatalf(
				"claim worker %d returned status %q",
				index+1,
				outcome.settlement.Status,
			)
		}

		if outcome.settlement.AttemptCount != 1 {
			t.Fatalf(
				"claim worker %d returned attempt_count=%d",
				index+1,
				outcome.settlement.AttemptCount,
			)
		}

		if outcome.claimed {
			successfulClaims++
		}
	}

	if successfulClaims != 1 {
		t.Fatalf(
			"expected exactly one successful claim, got %d",
			successfulClaims,
		)
	}

	// Once PROCESSING, another request must not reclaim it.
	repeated, claimed, err :=
		postgres.ClaimFlexSettlementProcessing(
			ctx,
			db,
			settlement.ID,
		)

	if err != nil {
		t.Fatalf("repeat processing claim: %v", err)
	}

	if claimed {
		t.Fatal("PROCESSING settlement was claimed again")
	}

	if repeated == nil ||
		repeated.Status != "PROCESSING" ||
		repeated.AttemptCount != 1 {
		t.Fatalf(
			"unexpected repeated claim result: %+v",
			repeated,
		)
	}

	var persistedStatus string
	var persistedAttempts int
	var settlementCount int

	if err := db.QueryRow(ctx, `
        SELECT status, attempt_count
        FROM flex_settlements
        WHERE id = $1
    `, settlement.ID).Scan(
		&persistedStatus,
		&persistedAttempts,
	); err != nil {
		t.Fatalf("read persisted settlement claim: %v", err)
	}

	if persistedStatus != "PROCESSING" ||
		persistedAttempts != 1 {
		t.Fatalf(
			"invalid persisted claim: status=%s attempts=%d",
			persistedStatus,
			persistedAttempts,
		)
	}

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_settlements
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&settlementCount); err != nil {
		t.Fatalf("count settlements after claims: %v", err)
	}

	if settlementCount != 1 {
		t.Fatalf(
			"expected one settlement, got %d",
			settlementCount,
		)
	}
}

// TestFlexSettlementCapturePreparation verifies atomic local financial
// record preparation without executing an external provider capture.
func TestFlexSettlementCapturePreparation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify settlement database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing capture preparation test on %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)

	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	// Capture preparation must reject unclaimed settlements.
	if _, err := postgres.PrepareFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
	); err == nil {
		t.Fatal("unclaimed settlement accepted capture preparation")
	}

	var beforePayments, beforeTransactions int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payments
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&beforePayments); err != nil {
		t.Fatalf("count payments before claim: %v", err)
	}

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE idempotency_key = $1
    `, settlement.CaptureIdempotencyKey).
		Scan(&beforeTransactions); err != nil {
		t.Fatalf("count transactions before claim: %v", err)
	}

	if beforePayments != 0 || beforeTransactions != 0 {
		t.Fatalf(
			"unclaimed preparation created records: payments=%d transactions=%d",
			beforePayments,
			beforeTransactions,
		)
	}

	claimedSettlement, claimed, err :=
		postgres.ClaimFlexSettlementProcessing(
			ctx,
			db,
			settlement.ID,
		)

	if err != nil {
		t.Fatalf("claim settlement: %v", err)
	}

	if !claimed || claimedSettlement.Status != "PROCESSING" {
		t.Fatalf(
			"expected successful PROCESSING claim: %+v claimed=%v",
			claimedSettlement,
			claimed,
		)
	}

	first, err := postgres.PrepareFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
	)

	if err != nil {
		t.Fatalf("prepare FLEX capture: %v", err)
	}

	if first == nil ||
		first.PaymentID == "" ||
		first.CaptureTransactionID == "" {
		t.Fatalf("incomplete capture preparation: %+v", first)
	}

	if first.SettlementID != settlement.ID ||
		first.CaptureIdempotencyKey != settlement.CaptureIdempotencyKey ||
		first.ProviderAuthorizationID != settlement.ProviderAuthorizationID {
		t.Fatalf("capture preparation identity mismatch: %+v", first)
	}

	// Repeated preparation must reuse both financial records.
	second, err := postgres.PrepareFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
	)

	if err != nil {
		t.Fatalf("repeat FLEX capture preparation: %v", err)
	}

	if second.PaymentID != first.PaymentID ||
		second.CaptureTransactionID != first.CaptureTransactionID ||
		second.CaptureIdempotencyKey != first.CaptureIdempotencyKey {
		t.Fatalf(
			"repeated preparation changed financial identity: first=%+v second=%+v",
			first,
			second,
		)
	}

	var paymentCount int
	var paymentStatus, paymentMethod string

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*), MIN(status), MIN(payment_method)
        FROM payments
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&paymentCount,
		&paymentStatus,
		&paymentMethod,
	); err != nil {
		t.Fatalf("verify prepared payment: %v", err)
	}

	if paymentCount != 1 ||
		paymentStatus != "PENDING" ||
		paymentMethod != "CARD" {
		t.Fatalf(
			"unexpected payment: count=%d status=%s method=%s",
			paymentCount,
			paymentStatus,
			paymentMethod,
		)
	}

	var transactionCount int
	var transactionStatus, transactionType string

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*), MIN(status), MIN(transaction_type)
        FROM payment_transactions
        WHERE payment_id = $1
          AND provider = 'STRIPE'
          AND idempotency_key = $2
    `, first.PaymentID, settlement.CaptureIdempotencyKey).Scan(
		&transactionCount,
		&transactionStatus,
		&transactionType,
	); err != nil {
		t.Fatalf("verify prepared capture transaction: %v", err)
	}

	if transactionCount != 1 ||
		transactionStatus != "PENDING" ||
		transactionType != "CAPTURE" {
		t.Fatalf(
			"unexpected transaction: count=%d status=%s type=%s",
			transactionCount,
			transactionStatus,
			transactionType,
		)
	}

	var linkedPaymentID, linkedTransactionID, settlementStatus string
	var attemptCount int

	if err := db.QueryRow(ctx, `
        SELECT payment_id,
               capture_transaction_id,
               status,
               attempt_count
        FROM flex_settlements
        WHERE id = $1
    `, settlement.ID).Scan(
		&linkedPaymentID,
		&linkedTransactionID,
		&settlementStatus,
		&attemptCount,
	); err != nil {
		t.Fatalf("verify settlement financial links: %v", err)
	}

	if linkedPaymentID != first.PaymentID ||
		linkedTransactionID != first.CaptureTransactionID ||
		settlementStatus != "PROCESSING" ||
		attemptCount != 1 {
		t.Fatalf(
			"unexpected settlement links: payment=%s transaction=%s status=%s attempts=%d",
			linkedPaymentID,
			linkedTransactionID,
			settlementStatus,
			attemptCount,
		)
	}
}

// TestFlexSettlementConcurrentCapturePreparation verifies that
// simultaneous preparation requests reuse one authoritative payment
// and one CAPTURE transaction.
func TestFlexSettlementConcurrentCapturePreparation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("refusing test on database %q", databaseName)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx,
		db,
		settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	const workers = 8

	type result struct {
		preparation *postgres.FlexCapturePreparation
		err         error
	}

	results := make(chan result, workers)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			preparation, err := postgres.PrepareFlexSettlementCapture(
				ctx,
				db,
				settlement.ID,
			)

			results <- result{
				preparation: preparation,
				err:         err,
			}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var expectedPaymentID string
	var expectedTransactionID string
	successful := 0

	for outcome := range results {
		if outcome.err != nil {
			t.Errorf("concurrent preparation failed: %v", outcome.err)
			continue
		}

		p := outcome.preparation
		if p == nil {
			t.Error("concurrent preparation returned nil result")
			continue
		}

		if expectedPaymentID == "" {
			expectedPaymentID = p.PaymentID
			expectedTransactionID = p.CaptureTransactionID
		}

		if p.PaymentID != expectedPaymentID ||
			p.CaptureTransactionID != expectedTransactionID ||
			p.SettlementID != settlement.ID ||
			p.CaptureIdempotencyKey != settlement.CaptureIdempotencyKey ||
			p.ProviderAuthorizationID != settlement.ProviderAuthorizationID {
			t.Errorf("concurrent preparation identity mismatch: %+v", p)
		}

		successful++
	}

	if successful != workers {
		t.Fatalf(
			"expected %d successful preparations, got %d",
			workers,
			successful,
		)
	}

	var paymentCount int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payments
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&paymentCount); err != nil {
		t.Fatalf("count payments: %v", err)
	}

	if paymentCount != 1 {
		t.Fatalf("expected one payment, got %d", paymentCount)
	}

	var transactionCount int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE provider = 'STRIPE'
          AND idempotency_key = $1
          AND transaction_type = 'CAPTURE'
    `, settlement.CaptureIdempotencyKey).
		Scan(&transactionCount); err != nil {
		t.Fatalf("count capture transactions: %v", err)
	}

	if transactionCount != 1 {
		t.Fatalf(
			"expected one CAPTURE transaction, got %d",
			transactionCount,
		)
	}

	var linkedPaymentID, linkedTransactionID, status string
	var attemptCount int

	if err := db.QueryRow(ctx, `
        SELECT payment_id,
               capture_transaction_id,
               status,
               attempt_count
        FROM flex_settlements
        WHERE id = $1
    `, settlement.ID).Scan(
		&linkedPaymentID,
		&linkedTransactionID,
		&status,
		&attemptCount,
	); err != nil {
		t.Fatalf("verify settlement links: %v", err)
	}

	if linkedPaymentID != expectedPaymentID ||
		linkedTransactionID != expectedTransactionID ||
		status != "PROCESSING" ||
		attemptCount != 1 {
		t.Fatalf(
			"invalid settlement state: payment=%s transaction=%s status=%s attempts=%d",
			linkedPaymentID,
			linkedTransactionID,
			status,
			attemptCount,
		)
	}
}

// TestFlexSettlementCapturePreparationRollback verifies that
// conflicting provider idempotency identity cannot create or link
// financial records for a FLEX settlement.
func TestFlexSettlementCapturePreparationRollback(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("refusing test on database %q", databaseName)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx,
		db,
		settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	// Create an authoritative payment for the completed trip.
	// The preparation operation must reuse this record.
	payment, err := postgres.NewPaymentRepository(db).
		CreateFromCompletedTrip(ctx, lifecycle.TripID, "CARD")

	if err != nil {
		t.Fatalf("create payment fixture: %v", err)
	}

	// Deliberately reserve the settlement's provider idempotency
	// key for a different operation type. Preparation must reject it.
	_, err = db.Exec(ctx, `
        INSERT INTO payment_transactions (
            payment_id,
            transaction_reference,
            provider,
            idempotency_key,
            transaction_type,
            status,
            amount,
            currency
        )
        VALUES (
            $1,
            $2,
            'STRIPE',
            $3,
            'AUTHORIZE',
            'PENDING',
            $4::numeric,
            $5
        )
    `,
		payment.ID,
		"conflict:"+settlement.ID,
		settlement.CaptureIdempotencyKey,
		payment.Amount,
		payment.Currency,
	)
	if err != nil {
		t.Fatalf("create conflicting transaction: %v", err)
	}

	preparation, err := postgres.PrepareFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
	)

	if err == nil {
		t.Fatalf(
			"expected conflicting transaction rejection, got %+v",
			preparation,
		)
	}

	if preparation != nil {
		t.Fatalf(
			"failed preparation returned financial records: %+v",
			preparation,
		)
	}

	var paymentCount int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payments
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&paymentCount); err != nil {
		t.Fatalf("count payments: %v", err)
	}

	if paymentCount != 1 {
		t.Fatalf("expected existing payment only, got %d", paymentCount)
	}

	var captureCount int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE payment_id = $1
          AND transaction_type = 'CAPTURE'
    `, payment.ID).Scan(&captureCount); err != nil {
		t.Fatalf("count CAPTURE transactions: %v", err)
	}

	if captureCount != 0 {
		t.Fatalf(
			"failed preparation created %d CAPTURE transactions",
			captureCount,
		)
	}

	var linkedPaymentID, linkedTransactionID *string
	var settlementStatus string

	if err := db.QueryRow(ctx, `
        SELECT payment_id,
               capture_transaction_id,
               status
        FROM flex_settlements
        WHERE id = $1
    `, settlement.ID).Scan(
		&linkedPaymentID,
		&linkedTransactionID,
		&settlementStatus,
	); err != nil {
		t.Fatalf("verify settlement rollback: %v", err)
	}

	if linkedPaymentID != nil ||
		linkedTransactionID != nil ||
		settlementStatus != "PROCESSING" {
		t.Fatalf(
			"failed preparation modified settlement: payment=%v transaction=%v status=%s",
			linkedPaymentID,
			linkedTransactionID,
			settlementStatus,
		)
	}
}

// TestFlexSettlementPaymentCreationRollback verifies that a payment
// inserted during preparation is rolled back when CAPTURE transaction
// creation fails due to a conflicting transaction reference.
func TestFlexSettlementPaymentCreationRollback(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("refusing test on database %q", databaseName)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx,
		db,
		lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx,
		db,
		settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	// PostgreSQL requires a payment for a payment transaction.
	// Create a fixture payment for this completed trip, then reserve
	// the reference that preparation would otherwise use.
	//
	// The fixture payment will be removed before preparation using
	// an isolated transaction with a deferred foreign key check
	// only if the schema permits it. The test instead uses a
	// separate completed FLEX lifecycle to supply the conflict.
	other := completeAuthorizedFlexLifecycle(t)

	otherPayment, err := postgres.NewPaymentRepository(db).
		CreateFromCompletedTrip(ctx, other.TripID, "CARD")
	if err != nil {
		t.Fatalf("create other payment: %v", err)
	}

	// Reserve the target settlement's transaction reference using
	// a different payment, but a distinct provider idempotency key.
	_, err = db.Exec(ctx, `
        INSERT INTO payment_transactions (
            payment_id,
            transaction_reference,
            provider,
            idempotency_key,
            transaction_type,
            status,
            amount,
            currency
        )
        VALUES (
            $1,
            $2,
            'STRIPE',
            $3,
            'CAPTURE',
            'PENDING',
            $4::numeric,
            $5
        )
    `,
		otherPayment.ID,
		"flex-capture:"+settlement.ID,
		"test-reference-conflict:"+settlement.ID,
		otherPayment.Amount,
		otherPayment.Currency,
	)
	if err != nil {
		t.Fatalf("reserve conflicting reference: %v", err)
	}

	var paymentsBefore int
	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payments
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&paymentsBefore); err != nil {
		t.Fatalf("count payments before preparation: %v", err)
	}

	if paymentsBefore != 0 {
		t.Fatalf(
			"expected no target payment before preparation, got %d",
			paymentsBefore,
		)
	}

	preparation, err := postgres.PrepareFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
	)

	if err == nil || preparation != nil {
		t.Fatalf(
			"expected transaction reference conflict: result=%+v err=%v",
			preparation,
			err,
		)
	}

	// The payment inserted before the failed CAPTURE insert
	// must not survive the transaction rollback.
	var paymentsAfter int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payments
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(&paymentsAfter); err != nil {
		t.Fatalf("count payments after rollback: %v", err)
	}

	if paymentsAfter != 0 {
		t.Fatalf(
			"failed preparation left %d newly created payments",
			paymentsAfter,
		)
	}

	var targetCaptureCount int

	if err := db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM payment_transactions
        WHERE provider = 'STRIPE'
          AND idempotency_key = $1
    `, settlement.CaptureIdempotencyKey).
		Scan(&targetCaptureCount); err != nil {
		t.Fatalf("count target CAPTURE transactions: %v", err)
	}

	if targetCaptureCount != 0 {
		t.Fatalf(
			"failed preparation created %d target transactions",
			targetCaptureCount,
		)
	}

	var linkedPaymentID, linkedTransactionID *string
	var status string
	var attempts int

	if err := db.QueryRow(ctx, `
        SELECT payment_id,
               capture_transaction_id,
               status,
               attempt_count
        FROM flex_settlements
        WHERE id = $1
    `, settlement.ID).Scan(
		&linkedPaymentID,
		&linkedTransactionID,
		&status,
		&attempts,
	); err != nil {
		t.Fatalf("verify settlement rollback: %v", err)
	}

	if linkedPaymentID != nil ||
		linkedTransactionID != nil ||
		status != "PROCESSING" ||
		attempts != 1 {
		t.Fatalf(
			"unexpected settlement after rollback: payment=%v transaction=%v status=%s attempts=%d",
			linkedPaymentID,
			linkedTransactionID,
			status,
			attempts,
		)
	}
}

// TestFlexSettlementDriverEarningsCommission verifies the authoritative
// FLEX commission and idempotent earnings creation without contacting Stripe.
func TestFlexSettlementDriverEarningsCommission(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)

	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing FLEX earnings test on database %q",
			databaseName,
		)
	}

	payment, err := postgres.NewPaymentRepository(db).
		CreateFromCompletedTrip(ctx, lifecycle.TripID, "CARD")
	if err != nil {
		t.Fatalf("create completed FLEX payment: %v", err)
	}

	if err := postgres.NewPaymentRepository(db).
		UpdateStatus(ctx, payment.ID, "PAID"); err != nil {
		t.Fatalf("mark isolated test payment PAID: %v", err)
	}

	earnings := postgres.NewDriverEarningRepository(db)

	earning, err := earnings.CreateFromPaidPayment(
		ctx,
		payment.ID,
	)
	if err != nil {
		t.Fatalf("create FLEX driver earning: %v", err)
	}

	if earning == nil {
		t.Fatal("FLEX driver earning is nil")
	}

	grossCents := lifecycle.AmountCents
	expectedCommissionCents := (grossCents*7 + 50) / 100
	expectedNetCents := grossCents - expectedCommissionCents

	var actualGrossCents int64
	var actualCommissionCents int64
	var actualNetCents int64

	err = db.QueryRow(ctx, `
        SELECT
            (gross_amount * 100)::bigint,
            (commission_amount * 100)::bigint,
            (net_amount * 100)::bigint
        FROM driver_earnings
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&actualGrossCents,
		&actualCommissionCents,
		&actualNetCents,
	)
	if err != nil {
		t.Fatalf("read FLEX earning amounts: %v", err)
	}

	if actualGrossCents != grossCents ||
		actualCommissionCents != expectedCommissionCents ||
		actualNetCents != expectedNetCents {

		t.Fatalf(
			"incorrect FLEX allocation: gross=%d commission=%d net=%d; "+
				"expected gross=%d commission=%d net=%d",
			actualGrossCents,
			actualCommissionCents,
			actualNetCents,
			grossCents,
			expectedCommissionCents,
			expectedNetCents,
		)
	}

	if actualGrossCents !=
		actualCommissionCents+actualNetCents {
		t.Fatal("FLEX allocation does not reconcile")
	}

	if earning.TripID != lifecycle.TripID ||
		earning.PaymentID == nil ||
		*earning.PaymentID != payment.ID {
		t.Fatal("FLEX earning financial identity mismatch")
	}

	replayed, err := earnings.CreateFromPaidPayment(
		ctx,
		payment.ID,
	)
	if err != nil {
		t.Fatalf("replay FLEX earning creation: %v", err)
	}

	if replayed == nil || replayed.ID != earning.ID {
		t.Fatal("FLEX earning replay changed identity")
	}

	var earningCount int
	var replayCommissionCents int64
	var replayNetCents int64

	err = db.QueryRow(ctx, `
        SELECT
            COUNT(*),
            COALESCE(MAX((commission_amount * 100)::bigint), 0),
            COALESCE(MAX((net_amount * 100)::bigint), 0)
        FROM driver_earnings
        WHERE trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningCount,
		&replayCommissionCents,
		&replayNetCents,
	)
	if err != nil {
		t.Fatalf("verify FLEX earning replay: %v", err)
	}

	if earningCount != 1 ||
		replayCommissionCents != expectedCommissionCents ||
		replayNetCents != expectedNetCents {
		t.Fatalf(
			"FLEX earning replay violated idempotency: "+
				"count=%d commission=%d net=%d",
			earningCount,
			replayCommissionCents,
			replayNetCents,
		)
	}
}

// TestFlexSettlementCommissionRounding verifies that PostgreSQL's
// FLEX commission rounding agrees with integer-cent accounting.
func TestFlexSettlementCommissionRounding(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify isolated database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing rounding test on database %q",
			databaseName,
		)
	}

	cases := []struct {
		name            string
		grossCents      int64
		commissionCents int64
		netCents        int64
	}{
		{"ten euros", 1000, 70, 930},
		{"ten euros five cents", 1005, 70, 935},
		{"twenty euros", 2000, 140, 1860},
		{"one cent", 1, 0, 1},
		{"eight cents rounds up", 8, 1, 7},
		{"fifty cents rounds up", 50, 4, 46},
		{"one euro", 100, 7, 93},
		{"twenty three euros twenty eight cents", 2328, 163, 2165},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gross, commission, net int64

			err := db.QueryRow(ctx, `
                WITH amount AS (
                    SELECT ($1::bigint::numeric / 100) AS gross
                )
                SELECT
                    (gross * 100)::bigint,
                    (ROUND(gross * 0.07, 2) * 100)::bigint,
                    ((gross - ROUND(gross * 0.07, 2)) * 100)::bigint
                FROM amount
            `, tc.grossCents).Scan(
				&gross,
				&commission,
				&net,
			)
			if err != nil {
				t.Fatalf("calculate FLEX commission: %v", err)
			}

			if gross != tc.grossCents ||
				commission != tc.commissionCents ||
				net != tc.netCents {
				t.Fatalf(
					"incorrect allocation: gross=%d commission=%d net=%d; "+
						"expected gross=%d commission=%d net=%d",
					gross,
					commission,
					net,
					tc.grossCents,
					tc.commissionCents,
					tc.netCents,
				)
			}

			if gross != commission+net {
				t.Fatal("commission and driver net do not reconcile")
			}

			expectedCommission := (tc.grossCents*7 + 50) / 100
			if commission != expectedCommission {
				t.Fatalf(
					"rounding mismatch: PostgreSQL=%d integer=%d",
					commission,
					expectedCommission,
				)
			}
		})
	}
}

// TestFlexSettlementAtomicCaptureFinalization verifies that a trusted
// successful capture result is persisted atomically and idempotently.
// This test simulates provider success; it does not contact Stripe.
func TestFlexSettlementAtomicCaptureFinalization(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing finalization test on database %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx, db, lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil {
		t.Fatalf("claim FLEX settlement: %v", err)
	}
	if !claimed {
		t.Fatal("expected FLEX processing claim")
	}

	preparation, err := postgres.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	)
	if err != nil {
		t.Fatalf("prepare FLEX capture: %v", err)
	}

	if preparation.ProviderAuthorizationID != lifecycle.ProviderID {
		t.Fatal("FLEX provider authorization identity mismatch")
	}

	if err := postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, lifecycle.ProviderID,
	); err != nil {
		t.Fatalf("finalize FLEX capture: %v", err)
	}

	var settlementStatus string
	var paymentStatus string
	var captureStatus string
	var persistedProviderCaptureID string
	var earningID string
	var grossCents int64
	var commissionCents int64
	var netCents int64

	err = db.QueryRow(ctx, `
        SELECT
            fs.status,
            p.status,
            pt.status,
            fs.provider_capture_id,
            de.id::text,
            (de.gross_amount * 100)::bigint,
            (de.commission_amount * 100)::bigint,
            (de.net_amount * 100)::bigint
        FROM flex_settlements fs
        JOIN payments p
            ON p.id = fs.payment_id
        JOIN payment_transactions pt
            ON pt.id = fs.capture_transaction_id
        JOIN driver_earnings de
            ON de.trip_id = fs.trip_id
        WHERE fs.id = $1
    `, settlement.ID).Scan(
		&settlementStatus,
		&paymentStatus,
		&captureStatus,
		&persistedProviderCaptureID,
		&earningID,
		&grossCents,
		&commissionCents,
		&netCents,
	)
	if err != nil {
		t.Fatalf("read finalized financial records: %v", err)
	}

	expectedCommission := (lifecycle.AmountCents*7 + 50) / 100

	if settlementStatus != "CAPTURED" ||
		paymentStatus != "PAID" ||
		captureStatus != "SUCCESS" ||
		persistedProviderCaptureID != lifecycle.ProviderID ||
		grossCents != lifecycle.AmountCents ||
		commissionCents != expectedCommission ||
		netCents != lifecycle.AmountCents-expectedCommission {
		t.Fatalf(
			"incorrect finalized FLEX state: "+
				"settlement=%s payment=%s capture=%s "+
				"provider=%s gross=%d commission=%d net=%d",
			settlementStatus,
			paymentStatus,
			captureStatus,
			persistedProviderCaptureID,
			grossCents,
			commissionCents,
			netCents,
		)
	}

	if err := postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, lifecycle.ProviderID,
	); err != nil {
		t.Fatalf("replay FLEX finalization: %v", err)
	}

	var earningCount int
	var replayEarningID string
	var replaySettlementStatus string

	err = db.QueryRow(ctx, `
        SELECT
            fs.status,
            de.id::text,
            (
                SELECT COUNT(*)
                FROM driver_earnings
                WHERE trip_id = fs.trip_id
            )
        FROM flex_settlements fs
        JOIN driver_earnings de
            ON de.trip_id = fs.trip_id
        WHERE fs.id = $1
    `, settlement.ID).Scan(
		&replaySettlementStatus,
		&replayEarningID,
		&earningCount,
	)
	if err != nil {
		t.Fatalf("verify finalization replay: %v", err)
	}

	if replaySettlementStatus != "CAPTURED" ||
		replayEarningID != earningID ||
		earningCount != 1 {
		t.Fatalf(
			"FLEX finalization replay violated idempotency: "+
				"status=%s earning=%s count=%d",
			replaySettlementStatus,
			replayEarningID,
			earningCount,
		)
	}
}

// TestFlexSettlementFinalizationRollback verifies that an earnings
// persistence failure rolls back the capture transaction, payment,
// and settlement updates together.
func TestFlexSettlementFinalizationRollback(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify isolated database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing rollback test on database %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx, db, lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim FLEX settlement: claimed=%v err=%v",
			claimed, err,
		)
	}

	preparation, err := postgres.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	)
	if err != nil {
		t.Fatalf("prepare FLEX capture: %v", err)
	}

	// Force driver earnings creation to fail through an existing
	// financial constraint. The completed trip remains unchanged.
	//
	// The trigger is transaction-local and only rejects INSERT into
	// driver_earnings for this specific trip.
	triggerName := "test_flex_earning_failure_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	_, err = db.Exec(ctx, fmt.Sprintf(`
        CREATE FUNCTION %s()
        RETURNS trigger
        LANGUAGE plpgsql
        AS $$
        BEGIN
            IF NEW.trip_id = '%s'::uuid THEN
                RAISE EXCEPTION 'forced FLEX earnings failure';
            END IF;
            RETURN NEW;
        END;
        $$;
    `, triggerName, lifecycle.TripID))
	if err != nil {
		t.Fatalf("install earnings failure function: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec(
			context.Background(),
			fmt.Sprintf(
				"DROP FUNCTION IF EXISTS %s() CASCADE",
				triggerName,
			),
		)
	})

	_, err = db.Exec(ctx, fmt.Sprintf(`
        CREATE TRIGGER %s
        BEFORE INSERT ON driver_earnings
        FOR EACH ROW
        EXECUTE FUNCTION %s()
    `, triggerName, triggerName))
	if err != nil {
		t.Fatalf("install earnings failure trigger: %v", err)
	}

	err = postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, lifecycle.ProviderID,
	)
	if err == nil {
		t.Fatal("expected forced earnings persistence failure")
	}

	var settlementStatus string
	var paymentStatus string
	var captureStatus string
	var providerCaptureID *string
	var capturedAt *time.Time
	var earningCount int

	err = db.QueryRow(ctx, `
        SELECT
            fs.status,
            p.status,
            pt.status,
            fs.provider_capture_id,
            fs.captured_at,
            (
                SELECT COUNT(*)
                FROM driver_earnings
                WHERE trip_id = fs.trip_id
            )
        FROM flex_settlements fs
        JOIN payments p ON p.id = fs.payment_id
        JOIN payment_transactions pt
            ON pt.id = fs.capture_transaction_id
        WHERE fs.id = $1
    `, settlement.ID).Scan(
		&settlementStatus,
		&paymentStatus,
		&captureStatus,
		&providerCaptureID,
		&capturedAt,
		&earningCount,
	)
	if err != nil {
		t.Fatalf("inspect rolled-back financial state: %v", err)
	}

	if settlementStatus != "PROCESSING" ||
		paymentStatus != "PENDING" ||
		captureStatus != "PENDING" ||
		providerCaptureID != nil ||
		capturedAt != nil ||
		earningCount != 0 {
		t.Fatalf(
			"partial FLEX financial commit detected: "+
				"settlement=%s payment=%s capture=%s "+
				"provider=%v capturedAt=%v earnings=%d",
			settlementStatus,
			paymentStatus,
			captureStatus,
			providerCaptureID,
			capturedAt,
			earningCount,
		)
	}

	// Remove the failure trigger, then prove the same prepared
	// capture can be finalized successfully.
	_, err = db.Exec(
		ctx,
		fmt.Sprintf("DROP TRIGGER %s ON driver_earnings", triggerName),
	)
	if err != nil {
		t.Fatalf("remove earnings failure trigger: %v", err)
	}

	_, err = db.Exec(
		ctx,
		fmt.Sprintf("DROP FUNCTION %s()", triggerName),
	)
	if err != nil {
		t.Fatalf("remove earnings failure function: %v", err)
	}

	if err := postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, lifecycle.ProviderID,
	); err != nil {
		t.Fatalf("finalize after rollback recovery: %v", err)
	}

	_ = preparation
}

// TestFlexSettlementConcurrentFinalization verifies that simultaneous
// delivery of the same verified provider capture result produces one
// settlement, one successful capture transaction, and one earning.
func TestFlexSettlementConcurrentFinalization(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify isolated database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing concurrency test on database %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx, db, lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim FLEX settlement: claimed=%v err=%v",
			claimed, err,
		)
	}

	preparation, err := postgres.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	)
	if err != nil {
		t.Fatalf("prepare FLEX capture: %v", err)
	}

	if preparation.ProviderAuthorizationID != lifecycle.ProviderID {
		t.Fatal("provider authorization identity mismatch")
	}

	const workers = 8

	start := make(chan struct{})
	results := make(chan error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			results <- postgres.FinalizeFlexSettlementCapture(
				ctx,
				db,
				settlement.ID,
				lifecycle.ProviderID,
			)
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf(
				"concurrent FLEX finalization failed: %v",
				err,
			)
		}
	}

	var settlementStatus string
	var paymentStatus string
	var captureStatus string
	var providerCaptureID string
	var earningCount int
	var captureCount int
	var grossCents int64
	var commissionCents int64
	var netCents int64

	err = db.QueryRow(ctx, `
        SELECT
            fs.status,
            p.status,
            pt.status,
            fs.provider_capture_id,
            (
                SELECT COUNT(*)
                FROM driver_earnings de
                WHERE de.trip_id = fs.trip_id
            ),
            (
                SELECT COUNT(*)
                FROM payment_transactions pt2
                WHERE pt2.payment_id = p.id
                  AND pt2.transaction_type = 'CAPTURE'
            ),
            (de.gross_amount * 100)::bigint,
            (de.commission_amount * 100)::bigint,
            (de.net_amount * 100)::bigint
        FROM flex_settlements fs
        JOIN payments p
            ON p.id = fs.payment_id
        JOIN payment_transactions pt
            ON pt.id = fs.capture_transaction_id
        JOIN driver_earnings de
            ON de.trip_id = fs.trip_id
        WHERE fs.id = $1
    `, settlement.ID).Scan(
		&settlementStatus,
		&paymentStatus,
		&captureStatus,
		&providerCaptureID,
		&earningCount,
		&captureCount,
		&grossCents,
		&commissionCents,
		&netCents,
	)
	if err != nil {
		t.Fatalf("read concurrent financial outcome: %v", err)
	}

	expectedCommission := (lifecycle.AmountCents*7 + 50) / 100

	if settlementStatus != "CAPTURED" ||
		paymentStatus != "PAID" ||
		captureStatus != "SUCCESS" ||
		providerCaptureID != lifecycle.ProviderID ||
		earningCount != 1 ||
		captureCount != 1 ||
		grossCents != lifecycle.AmountCents ||
		commissionCents != expectedCommission ||
		netCents != lifecycle.AmountCents-expectedCommission {
		t.Fatalf(
			"invalid concurrent FLEX outcome: "+
				"settlement=%s payment=%s capture=%s "+
				"provider=%s earnings=%d captures=%d "+
				"gross=%d commission=%d net=%d",
			settlementStatus,
			paymentStatus,
			captureStatus,
			providerCaptureID,
			earningCount,
			captureCount,
			grossCents,
			commissionCents,
			netCents,
		)
	}
}

// TestFlexSettlementCaptureProviderIdentityConflict verifies that
// finalization rejects a different provider capture identity both
// before and after successful capture without changing financial state.
func TestFlexSettlementCaptureProviderIdentityConflict(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	ctx := context.Background()
	db := lifecycle.Fixture.DB

	var databaseName string
	if err := db.QueryRow(
		ctx, "SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify isolated database: %v", err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf("refusing test on database %q", databaseName)
	}

	settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
		ctx, db, lifecycle.TripID,
	)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err := postgres.ClaimFlexSettlementProcessing(
		ctx, db, settlement.ID,
	)
	if err != nil || !claimed {
		t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
	}

	if _, err := postgres.PrepareFlexSettlementCapture(
		ctx, db, settlement.ID,
	); err != nil {
		t.Fatalf("prepare capture: %v", err)
	}

	conflictingID := lifecycle.ProviderID + "_conflict"

	// First reject the conflicting identity before capture.
	if err := postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, conflictingID,
	); err == nil {
		t.Fatal("conflicting provider identity accepted before capture")
	}

	var status, paymentStatus, captureStatus string
	var providerCaptureID *string
	var earningCount int

	checkState := func(
		expectedSettlement string,
		expectedPayment string,
		expectedCapture string,
		expectedProvider *string,
		expectedEarnings int,
	) {
		t.Helper()

		err := db.QueryRow(ctx, `
            SELECT
                fs.status,
                p.status,
                pt.status,
                fs.provider_capture_id,
                (
                    SELECT COUNT(*)
                    FROM driver_earnings
                    WHERE trip_id = fs.trip_id
                )
            FROM flex_settlements fs
            JOIN payments p ON p.id = fs.payment_id
            JOIN payment_transactions pt
                ON pt.id = fs.capture_transaction_id
            WHERE fs.id = $1
        `, settlement.ID).Scan(
			&status,
			&paymentStatus,
			&captureStatus,
			&providerCaptureID,
			&earningCount,
		)
		if err != nil {
			t.Fatalf("read financial state: %v", err)
		}

		providerMatches := (providerCaptureID == nil &&
			expectedProvider == nil) ||
			(providerCaptureID != nil &&
				expectedProvider != nil &&
				*providerCaptureID == *expectedProvider)

		if status != expectedSettlement ||
			paymentStatus != expectedPayment ||
			captureStatus != expectedCapture ||
			!providerMatches ||
			earningCount != expectedEarnings {
			t.Fatalf(
				"unexpected financial state: settlement=%s "+
					"payment=%s capture=%s provider=%v earnings=%d",
				status,
				paymentStatus,
				captureStatus,
				providerCaptureID,
				earningCount,
			)
		}
	}

	checkState("PROCESSING", "PENDING", "PENDING", nil, 0)

	// The correct authorized identity must still finalize.
	if err := postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, lifecycle.ProviderID,
	); err != nil {
		t.Fatalf("finalize correct provider identity: %v", err)
	}

	checkState(
		"CAPTURED",
		"PAID",
		"SUCCESS",
		&lifecycle.ProviderID,
		1,
	)

	// Reject a conflicting identity after finalization.
	if err := postgres.FinalizeFlexSettlementCapture(
		ctx, db, settlement.ID, conflictingID,
	); err == nil {
		t.Fatal("conflicting provider identity accepted after capture")
	}

	checkState(
		"CAPTURED",
		"PAID",
		"SUCCESS",
		&lifecycle.ProviderID,
		1,
	)
}
