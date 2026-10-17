package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

type flexAuthorizationTestFixture struct {
	*testutil.FlexAcceptFixture
	PassengerID string
	OfferID     string
}

func setupFlexAuthorizationTest(
	t *testing.T,
) *flexAuthorizationTestFixture {
	t.Helper()

	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := flex_offer.NewService(flex_offer.Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	offer, err := service.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	var passengerID string
	err = f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID)
	if err != nil {
		t.Fatalf("resolve passenger: %v", err)
	}

	_, err = service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offer.ID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("select FLEX offer: %v", err)
	}

	// Cleanup executes in reverse registration order.
	// Remove authorization before releasing the selected offer.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		_, err := f.DB.Exec(cleanupCtx, `
			DELETE FROM flex_authorizations
			WHERE auction_id = $1
		`, f.AuctionID)
		if err != nil {
			t.Errorf("cleanup FLEX authorization: %v", err)
		}

		_, err = f.DB.Exec(cleanupCtx, `
			UPDATE flex_auctions
			SET status = 'OPEN',
			    selected_offer_id = NULL,
			    resolved_at = NULL,
			    updated_at = clock_timestamp()
			WHERE id = $1
			  AND status = 'PENDING_AUTHORIZATION'
		`, f.AuctionID)
		if err != nil {
			t.Errorf("release FLEX selected offer: %v", err)
		}
	})

	return &flexAuthorizationTestFixture{
		FlexAcceptFixture: f,
		PassengerID:       passengerID,
		OfferID:           offer.ID,
	}
}

func createTestFlexAuthorization(
	ctx context.Context,
	f *flexAuthorizationTestFixture,
) (*postgresrepo.FlexAuthorization, error) {
	return postgresrepo.CreateFlexAuthorization(
		ctx,
		f.DB,
		f.RideID,
		f.AuctionID,
		f.PassengerID,
	)
}

func TestCreateFlexAuthorizationSuccess(t *testing.T) {
	f := setupFlexAuthorizationTest(t)

	authorization, err := createTestFlexAuthorization(
		context.Background(),
		f,
	)
	if err != nil {
		t.Fatalf("create FLEX authorization: %v", err)
	}

	if authorization.ID == "" ||
		authorization.AuctionID != f.AuctionID ||
		authorization.OfferID != f.OfferID ||
		authorization.PassengerID != f.PassengerID ||
		authorization.AmountCents != 2328 ||
		authorization.Currency != "EUR" ||
		authorization.Provider != "STRIPE" ||
		authorization.Status != "PENDING" ||
		authorization.IdempotencyKey != "flex-authorize:"+f.AuctionID {
		t.Fatalf("unexpected FLEX authorization: %+v", authorization)
	}

	var persistedAmount string
	err = f.DB.QueryRow(context.Background(), `
		SELECT amount::text
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&persistedAmount)
	if err != nil {
		t.Fatalf("read persisted amount: %v", err)
	}
	if persistedAmount != "23.28" {
		t.Fatalf("unexpected persisted amount: %s", persistedAmount)
	}
}

func TestCreateFlexAuthorizationRejectsNonOwner(t *testing.T) {
	f := setupFlexAuthorizationTest(t)

	_, err := postgresrepo.CreateFlexAuthorization(
		context.Background(),
		f.DB,
		f.RideID,
		f.AuctionID,
		uuid.NewString(),
	)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationAccessDenied) {
		t.Fatalf("expected access denied, got: %v", err)
	}

	assertFlexAuthorizationCount(t, f, 0)
}

func TestCreateFlexAuthorizationRejectsUnselectedAuction(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)

	var passengerID string
	err := f.DB.QueryRow(context.Background(), `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = postgresrepo.CreateFlexAuthorization(
		context.Background(),
		f.DB,
		f.RideID,
		f.AuctionID,
		passengerID,
	)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotEligible) {
		t.Fatalf("expected ineligible auction, got: %v", err)
	}

	var count int
	err = f.DB.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unexpected authorization count: %d", count)
	}
}

func TestCreateFlexAuthorizationIdempotentRetry(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	first, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	second, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	if first.ID != second.ID ||
		first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("retry changed authorization identity")
	}

	assertFlexAuthorizationCount(t, f, 1)
}

func TestCreateFlexAuthorizationConcurrentRetry(t *testing.T) {
	f := setupFlexAuthorizationTest(t)

	const workers = 4

	results := make([]*postgresrepo.FlexAuthorization, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			results[index], errs[index] =
				createTestFlexAuthorization(context.Background(), f)
		}(i)
	}

	wg.Wait()

	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d failed: %v", i, errs[i])
		}
		if results[i] == nil ||
			results[i].ID != results[0].ID {
			t.Fatalf("worker %d returned different authorization", i)
		}
	}

	assertFlexAuthorizationCount(t, f, 1)
}

func TestCreateFlexAuthorizationRollbackOnConflict(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	// Introduce an incompatible existing authorization.
	_, err := f.DB.Exec(ctx, `
		INSERT INTO flex_authorizations (
			auction_id,
			offer_id,
			passenger_id,
			amount,
			currency,
			idempotency_key,
			expires_at
		)
		VALUES (
			$1, $2, $3, 24.00, 'EUR', $4,
			clock_timestamp() + INTERVAL '5 minutes'
		)
	`,
		f.AuctionID,
		f.OfferID,
		f.PassengerID,
		"flex-authorize-conflict:"+f.AuctionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = createTestFlexAuthorization(ctx, f)
	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotEligible) {
		t.Fatalf("expected immutable authorization conflict, got: %v", err)
	}

	assertFlexAuthorizationCount(t, f, 1)
}

func assertFlexAuthorizationCount(
	t *testing.T,
	f *flexAuthorizationTestFixture,
	expected int,
) {
	t.Helper()

	var count int
	err := f.DB.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM flex_authorizations
		WHERE auction_id = $1
	`, f.AuctionID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != expected {
		t.Fatalf(
			"authorization count = %d, want %d",
			count,
			expected,
		)
	}
}

// TestFlexAuthorizationFinancialIdentityImmutable verifies that PostgreSQL,
// independently of application logic, rejects modifications to the
// authorization's financial and ownership identity.
func TestFlexAuthorizationFinancialIdentityImmutable(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		column string
		value  string
	}{
		{"amount", "amount", "24.00"},
		{"passenger", "passenger_id", uuid.NewString()},
		{"offer", "offer_id", uuid.NewString()},
		{"idempotency", "idempotency_key", "modified-key"},
		{"currency", "currency", "USD"},
		{"provider", "provider", "OTHER"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := "UPDATE flex_authorizations SET " +
				tc.column + " = $1 WHERE id = $2"

			_, err := f.DB.Exec(
				ctx,
				query,
				tc.value,
				authorization.ID,
			)
			if err == nil {
				t.Fatalf(
					"database permitted immutable field modification: %s",
					tc.column,
				)
			}
		})
	}

	reloaded, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	if reloaded.ID != authorization.ID ||
		reloaded.AmountCents != authorization.AmountCents ||
		reloaded.PassengerID != authorization.PassengerID ||
		reloaded.OfferID != authorization.OfferID ||
		reloaded.IdempotencyKey != authorization.IdempotencyKey {
		t.Fatal("authorization identity changed after rejected updates")
	}
}

// TestFlexAuthorizationRequiresExpiry verifies that new authorization
// records cannot be inserted without an initiation deadline.
func TestFlexAuthorizationRequiresExpiry(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	_, err := f.DB.Exec(ctx, `
		INSERT INTO flex_authorizations (
			auction_id,
			offer_id,
			passenger_id,
			amount,
			currency,
			idempotency_key
		)
		VALUES ($1, $2, $3, 23.28, 'EUR', $4)
	`,
		f.AuctionID,
		f.OfferID,
		f.PassengerID,
		"missing-expiry:"+f.AuctionID,
	)

	if err == nil {
		t.Fatal("database accepted authorization without expiry")
	}

	assertFlexAuthorizationCount(t, f, 0)
}

// TestFlexAuthorizationExpiryIsImmutable verifies that an existing
// initiation deadline cannot be extended by an ordinary UPDATE.
func TestFlexAuthorizationExpiryIsImmutable(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_authorizations
		SET expires_at = expires_at + INTERVAL '5 minutes'
		WHERE id = $1
	`, authorization.ID)

	if err == nil {
		t.Fatal("database permitted authorization deadline extension")
	}

	assertFlexAuthorizationCount(t, f, 1)
}
