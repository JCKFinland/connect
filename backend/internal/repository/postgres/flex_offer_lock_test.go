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

func TestFlexOfferRepositoryGetByIDForUpdateBlocksConcurrentTransaction(
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
		t.Fatalf("create isolated driver fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)
	rideRequestID := uuid.NewString()

	// Commit setup records so two independent transactions can
	// contend for the same offer row.
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
			'Flex Offer Lock Pickup', 60.2055, 24.6559,
			'Flex Offer Lock Destination', 60.1719, 24.9414,
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
		PassengerOfferCents: 1500,
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

	offer := &models.FlexOffer{
		AuctionID:          auction.ID,
		AssignmentID:       fixture.AssignmentID,
		DriverID:           fixture.UserID,
		VehicleID:          fixture.VehicleID,
		CompanyID:          fixture.CompanyID,
		BranchID:           fixture.BranchID,
		FleetID:            fixture.FleetID,
		OfferedAmountCents: 1250,
		OfferedAt:          now,
		ExpiresAt:          now.Add(5 * time.Minute),
	}
	if err := NewFlexOfferRepositoryWithDB(setupTx).Create(
		ctx,
		offer,
	); err != nil {
		t.Fatalf("create lock-test offer: %v", err)
	}

	if err := setupTx.Commit(ctx); err != nil {
		t.Fatalf("commit lock-test setup: %v", err)
	}
	setupCommitted = true

	// Cleanup runs after all locking transactions have been released.
	// Delete in foreign-key dependency order.
	defer func() {
		cleanupCtx := context.Background()

		for _, item := range []struct {
			query string
			id    string
		}{
			{`DELETE FROM flex_offers WHERE id = $1`, offer.ID},
			{`DELETE FROM flex_auctions WHERE id = $1`, auction.ID},
			{`DELETE FROM ride_requests WHERE id = $1`, rideRequestID},
		} {
			if _, cleanupErr := db.Exec(
				cleanupCtx,
				item.query,
				item.id,
			); cleanupErr != nil {
				t.Errorf("cleanup test record %s: %v", item.id, cleanupErr)
			}
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

	repo1 := NewFlexOfferRepositoryWithDB(tx1)
	repo2 := NewFlexOfferRepositoryWithDB(tx2)

	if _, err := repo1.GetByIDForUpdate(ctx, offer.ID); err != nil {
		t.Fatalf("first transaction failed to lock offer: %v", err)
	}

	_, err = tx2.Exec(ctx, `SET LOCAL lock_timeout = '250ms'`)
	if err != nil {
		t.Fatalf("configure competing transaction: %v", err)
	}

	_, err = repo2.GetByIDForUpdate(ctx, offer.ID)
	if err == nil {
		t.Fatal("second transaction unexpectedly acquired locked offer")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf(
			"expected lock_not_available (55P03), got: %v",
			err,
		)
	}

	if err := tx2.Rollback(ctx); err != nil {
		t.Fatalf("rollback competing transaction: %v", err)
	}
	if err := tx1.Rollback(ctx); err != nil {
		t.Fatalf("release first offer lock: %v", err)
	}

	// After the first transaction releases the lock, a new
	// transaction must be able to acquire it.
	tx3, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin verification transaction: %v", err)
	}
	defer func() {
		_ = tx3.Rollback(context.Background())
	}()

	locked, err := NewFlexOfferRepositoryWithDB(tx3).
		GetByIDForUpdate(ctx, offer.ID)
	if err != nil {
		t.Fatalf("acquire released offer lock: %v", err)
	}
	if locked.ID != offer.ID {
		t.Fatalf("locked unexpected offer: %s", locked.ID)
	}

	if err := tx3.Rollback(ctx); err != nil {
		t.Fatalf("release verification lock: %v", err)
	}
}
