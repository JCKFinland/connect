package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFlexAuctionRepositoryGetByIDForUpdateBlocksConcurrentTransaction(
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

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated identity fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(context.Background()); cleanupErr != nil {
			t.Logf("cleanup identity fixture: %v", cleanupErr)
		}
	}()

	// The fixture identity is committed. Create the test ride request
	// and auction in a setup transaction so both locking transactions
	// can see the same committed auction.
	setupTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin setup transaction: %v", err)
	}
	setupCommitted := false
	defer func() {
		if !setupCommitted {
			_ = setupTx.Rollback(context.Background())
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)
	rideRequestID := uuid.NewString()

	_, err = setupTx.Exec(
		ctx,
		`
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
			'Flex Lock Test Pickup', 60.2055, 24.6559,
			'Flex Lock Test Destination', 60.1719, 24.9414,
			'STANDARD', 1, 'PENDING', $3, $4, $3, $3
		)
		`,
		rideRequestID,
		fixture.UserID,
		now,
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("create lock-test ride request: %v", err)
	}

	auction := &models.FlexAuction{
		RideRequestID:       rideRequestID,
		PassengerOfferCents: 1250,
		Currency:            "EUR",
		OpenedAt:            now,
		ExpiresAt:           now.Add(10 * time.Minute),
	}

	if err := NewFlexAuctionRepositoryWithDB(setupTx).Create(
		ctx,
		auction,
	); err != nil {
		t.Fatalf("create lock-test auction: %v", err)
	}

	if err := setupTx.Commit(ctx); err != nil {
		t.Fatalf("commit lock-test setup: %v", err)
	}
	setupCommitted = true

	// Delete child first, then parent. Cleanup runs after both
	// locking transactions have been released.
	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM flex_auctions WHERE id = $1`,
			auction.ID,
		); cleanupErr != nil {
			t.Logf("cleanup flex auction: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf("cleanup ride request: %v", cleanupErr)
		}
	}()

	tx1, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first transaction: %v", err)
	}
	defer func() {
		_ = tx1.Rollback(context.Background())
	}()

	tx2, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second transaction: %v", err)
	}
	defer func() {
		_ = tx2.Rollback(context.Background())
	}()

	repo1 := NewFlexAuctionRepositoryWithDB(tx1)
	repo2 := NewFlexAuctionRepositoryWithDB(tx2)

	if _, err := repo1.GetByIDForUpdate(ctx, auction.ID); err != nil {
		t.Fatalf("first transaction failed to lock auction: %v", err)
	}

	// PostgreSQL's lock_timeout gives a deterministic assertion:
	// the second transaction cannot acquire the row lock while
	// the first transaction owns it.
	_, err = tx2.Exec(
		ctx,
		`SET LOCAL lock_timeout = '250ms'`,
	)
	if err != nil {
		t.Fatalf("set second transaction lock timeout: %v", err)
	}

	_, err = repo2.GetByIDForUpdate(ctx, auction.ID)
	if err == nil {
		t.Fatal("second transaction unexpectedly acquired locked auction")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf(
			"expected PostgreSQL lock_not_available (55P03), got: %v",
			err,
		)
	}

	// The lock-timeout error aborts tx2; release it and start a
	// fresh transaction after tx1 releases the auction.
	if err := tx2.Rollback(ctx); err != nil {
		t.Fatalf("rollback timed-out transaction: %v", err)
	}

	if err := tx1.Rollback(ctx); err != nil {
		t.Fatalf("release first transaction lock: %v", err)
	}

	tx3, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin third transaction: %v", err)
	}
	defer func() {
		_ = tx3.Rollback(context.Background())
	}()

	if _, err := NewFlexAuctionRepositoryWithDB(tx3).GetByIDForUpdate(
		ctx,
		auction.ID,
	); err != nil {
		t.Fatalf(
			"auction lock unavailable after first transaction released it: %v",
			err,
		)
	}

	if err := tx3.Rollback(ctx); err != nil {
		t.Fatalf("release third transaction lock: %v", err)
	}
}
