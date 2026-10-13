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
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFlexAuctionRepositoryPersistence(t *testing.T) {
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

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin test transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	// Use a savepoint so the expected duplicate-key error does not
	// invalidate the transaction used by subsequent assertions.
	fixture, cleanupFixture, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create isolated identity fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(
			context.Background(),
		); cleanupErr != nil {
			t.Logf("cleanup identity fixture: %v", cleanupErr)
		}
	}()

	rideRequestID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = tx.Exec(
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
			$1, $2,
			'Flex Repository Test Pickup',
			60.2055, 24.6559,
			'Flex Repository Test Destination',
			60.1719, 24.9414,
			'STANDARD', 1, 'PENDING',
			$3, $4, $3, $3
		)
		`,
		rideRequestID,
		fixture.UserID,
		now,
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("create transactional ride request: %v", err)
	}

	pricingProfileID, pricingVersion, err := createFlexPricingFixture(
		ctx,
		tx,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create FLEX pricing fixture: %v", err)
	}

	repo := NewFlexAuctionRepositoryWithDB(tx)

	// Deliberately supply hostile lifecycle fields. Create must
	// canonicalize the auction to the OPEN state.
	selectedOfferID := uuid.NewString()
	resolvedAt := now

	auction := &models.FlexAuction{
		RideRequestID:       rideRequestID,
		PassengerOfferCents: 12345,
		Currency:            "EUR",
		BaselineCents:       12345,
		MinimumOfferCents:   11110,
		MaximumOfferCents:   13580,
		PricingProfileID:    pricingProfileID,
		PricingVersion:      pricingVersion,
		Status:              models.FlexAuctionStatusConfirmed,
		SelectedOfferID:     &selectedOfferID,
		OpenedAt:            now,
		ExpiresAt:           now.Add(10 * time.Minute),
		ResolvedAt:          &resolvedAt,
	}

	if err := repo.Create(ctx, auction); err != nil {
		t.Fatalf("create flex auction: %v", err)
	}

	if auction.ID == "" {
		t.Fatal("expected generated auction ID")
	}

	if auction.Status != models.FlexAuctionStatusOpen ||
		auction.SelectedOfferID != nil ||
		auction.ResolvedAt != nil {
		t.Fatalf(
			"auction lifecycle not canonicalized: %+v",
			auction,
		)
	}

	byID, err := repo.GetByID(ctx, auction.ID)
	if err != nil {
		t.Fatalf("get auction by ID: %v", err)
	}

	if byID.PassengerOfferCents != 12345 {
		t.Fatalf(
			"monetary round-trip mismatch: got %d, want 12345",
			byID.PassengerOfferCents,
		)
	}

	assertPricingProvenance := func(
		label string,
		loaded *models.FlexAuction,
	) {
		t.Helper()

		if loaded.BaselineCents != auction.BaselineCents ||
			loaded.MinimumOfferCents != auction.MinimumOfferCents ||
			loaded.MaximumOfferCents != auction.MaximumOfferCents ||
			loaded.PricingProfileID != auction.PricingProfileID ||
			loaded.PricingVersion != auction.PricingVersion {
			t.Fatalf(
				"%s pricing provenance mismatch: got %+v, want %+v",
				label,
				loaded,
				auction,
			)
		}
	}

	assertPricingProvenance("GetByID", byID)

	if byID.Status != models.FlexAuctionStatusOpen ||
		byID.SelectedOfferID != nil ||
		byID.ResolvedAt != nil {
		t.Fatalf("persisted lifecycle is not OPEN: %+v", byID)
	}

	byRideRequest, err := repo.GetByRideRequestID(
		ctx,
		rideRequestID,
	)
	if err != nil {
		t.Fatalf("get auction by ride request: %v", err)
	}
	assertPricingProvenance("GetByRideRequestID", byRideRequest)

	if byRideRequest.ID != auction.ID {
		t.Fatalf(
			"ride request lookup returned %s, want %s",
			byRideRequest.ID,
			auction.ID,
		)
	}

	locked, err := repo.GetByIDForUpdate(ctx, auction.ID)
	if err != nil {
		t.Fatalf("lock auction: %v", err)
	}
	assertPricingProvenance("GetByIDForUpdate", locked)

	if locked.ID != auction.ID {
		t.Fatalf("locked unexpected auction: %s", locked.ID)
	}

	_, err = tx.Exec(ctx, "SAVEPOINT flex_duplicate_check")
	if err != nil {
		t.Fatalf("create duplicate-check savepoint: %v", err)
	}

	duplicate := &models.FlexAuction{
		RideRequestID:       rideRequestID,
		PassengerOfferCents: 15000,
		Currency:            "EUR",
		BaselineCents:       15000,
		MinimumOfferCents:   13500,
		MaximumOfferCents:   16500,
		PricingProfileID:    pricingProfileID,
		PricingVersion:      pricingVersion,
		OpenedAt:            now,
		ExpiresAt:           now.Add(10 * time.Minute),
	}

	err = repo.Create(ctx, duplicate)
	if err == nil {
		t.Fatal("expected duplicate ride-request auction rejection")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected unique violation 23505, got: %v", err)
	}

	_, err = tx.Exec(
		ctx,
		"ROLLBACK TO SAVEPOINT flex_duplicate_check",
	)
	if err != nil {
		t.Fatalf("rollback duplicate-check savepoint: %v", err)
	}

	_, err = tx.Exec(
		ctx,
		"RELEASE SAVEPOINT flex_duplicate_check",
	)
	if err != nil {
		t.Fatalf("release duplicate-check savepoint: %v", err)
	}

	_, err = repo.GetByID(ctx, uuid.NewString())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// Rollback is explicit so we can verify the auction did not persist.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback test transaction: %v", err)
	}

	_, err = NewFlexAuctionRepository(db).GetByID(
		ctx,
		auction.ID,
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected rolled-back auction to be absent, got: %v",
			err,
		)
	}
}
