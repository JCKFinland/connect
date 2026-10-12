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

func TestFlexOfferRepositoryPersistence(t *testing.T) {
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

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin test transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)
	rideRequestID := uuid.NewString()

	_, err = tx.Exec(
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
			'Flex Offer Test Pickup', 60.2055, 24.6559,
			'Flex Offer Test Destination', 60.1719, 24.9414,
			'STANDARD', 1, 'PENDING', $3, $4, $3, $3
		)
		`,
		rideRequestID,
		fixture.UserID,
		now,
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("create test ride request: %v", err)
	}

	auctionRepo := NewFlexAuctionRepositoryWithDB(tx)
	auction := &models.FlexAuction{
		RideRequestID:       rideRequestID,
		PassengerOfferCents: 1500,
		Currency:            "EUR",
		OpenedAt:            now,
		ExpiresAt:           now.Add(10 * time.Minute),
	}

	if err := auctionRepo.Create(ctx, auction); err != nil {
		t.Fatalf("create test auction: %v", err)
	}

	repo := NewFlexOfferRepositoryWithDB(tx)

	hostileRespondedAt := now
	offer := &models.FlexOffer{
		AuctionID:          auction.ID,
		AssignmentID:       fixture.AssignmentID,
		DriverID:           fixture.UserID,
		VehicleID:          fixture.VehicleID,
		CompanyID:          fixture.CompanyID,
		BranchID:           fixture.BranchID,
		FleetID:            fixture.FleetID,
		OfferedAmountCents: 12345,
		Status:             models.FlexOfferStatusAccepted,
		OfferedAt:          now,
		ExpiresAt:          now.Add(5 * time.Minute),
		RespondedAt:        &hostileRespondedAt,
	}

	if err := repo.Create(ctx, offer); err != nil {
		t.Fatalf("create flex offer: %v", err)
	}

	if offer.ID == "" {
		t.Fatal("expected generated offer ID")
	}

	if offer.Status != models.FlexOfferStatusPending ||
		offer.RespondedAt != nil {
		t.Fatalf("offer lifecycle not canonicalized: %+v", offer)
	}

	loaded, err := repo.GetByID(ctx, offer.ID)
	if err != nil {
		t.Fatalf("get flex offer: %v", err)
	}

	if loaded.OfferedAmountCents != 12345 {
		t.Fatalf(
			"amount round-trip mismatch: got %d, want 12345",
			loaded.OfferedAmountCents,
		)
	}

	if loaded.Status != models.FlexOfferStatusPending ||
		loaded.RespondedAt != nil {
		t.Fatalf("persisted offer lifecycle incorrect: %+v", loaded)
	}

	if loaded.AssignmentID != fixture.AssignmentID ||
		loaded.DriverID != fixture.UserID ||
		loaded.VehicleID != fixture.VehicleID ||
		loaded.CompanyID != fixture.CompanyID ||
		loaded.BranchID != fixture.BranchID ||
		loaded.FleetID != fixture.FleetID {
		t.Fatalf("assignment identity mismatch: %+v", loaded)
	}

	offers, err := repo.ListByAuctionID(ctx, auction.ID)
	if err != nil {
		t.Fatalf("list flex offers: %v", err)
	}
	if len(offers) != 1 || offers[0].ID != offer.ID {
		t.Fatalf("unexpected auction offers: %+v", offers)
	}

	locked, err := repo.GetByIDForUpdate(ctx, offer.ID)
	if err != nil {
		t.Fatalf("lock flex offer: %v", err)
	}
	if locked.ID != offer.ID {
		t.Fatalf("locked unexpected offer: %s", locked.ID)
	}

	// Duplicate driver bids must be rejected by the unique constraint.
	if _, err := tx.Exec(ctx, "SAVEPOINT flex_offer_duplicate"); err != nil {
		t.Fatalf("create duplicate savepoint: %v", err)
	}

	duplicate := &models.FlexOffer{
		AuctionID:          auction.ID,
		AssignmentID:       fixture.AssignmentID,
		DriverID:           fixture.UserID,
		VehicleID:          fixture.VehicleID,
		CompanyID:          fixture.CompanyID,
		BranchID:           fixture.BranchID,
		FleetID:            fixture.FleetID,
		OfferedAmountCents: 14000,
		OfferedAt:          now,
		ExpiresAt:          now.Add(5 * time.Minute),
	}

	err = repo.Create(ctx, duplicate)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected duplicate bid violation 23505, got: %v", err)
	}

	if _, err := tx.Exec(
		ctx,
		"ROLLBACK TO SAVEPOINT flex_offer_duplicate",
	); err != nil {
		t.Fatalf("rollback duplicate savepoint: %v", err)
	}
	if _, err := tx.Exec(
		ctx,
		"RELEASE SAVEPOINT flex_offer_duplicate",
	); err != nil {
		t.Fatalf("release duplicate savepoint: %v", err)
	}

	// The composite foreign key must reject an assignment paired with
	// a different driver identity.
	if _, err := tx.Exec(ctx, "SAVEPOINT flex_offer_identity"); err != nil {
		t.Fatalf("create identity savepoint: %v", err)
	}

	mismatched := &models.FlexOffer{
		AuctionID:          auction.ID,
		AssignmentID:       fixture.AssignmentID,
		DriverID:           uuid.NewString(),
		VehicleID:          fixture.VehicleID,
		CompanyID:          fixture.CompanyID,
		BranchID:           fixture.BranchID,
		FleetID:            fixture.FleetID,
		OfferedAmountCents: 15000,
		OfferedAt:          now,
		ExpiresAt:          now.Add(5 * time.Minute),
	}

	err = repo.Create(ctx, mismatched)
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("expected assignment FK violation 23503, got: %v", err)
	}

	if _, err := tx.Exec(
		ctx,
		"ROLLBACK TO SAVEPOINT flex_offer_identity",
	); err != nil {
		t.Fatalf("rollback identity savepoint: %v", err)
	}
	if _, err := tx.Exec(
		ctx,
		"RELEASE SAVEPOINT flex_offer_identity",
	); err != nil {
		t.Fatalf("release identity savepoint: %v", err)
	}

	_, err = repo.GetByID(ctx, uuid.NewString())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback test transaction: %v", err)
	}

	_, err = NewFlexOfferRepository(db).GetByID(ctx, offer.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected rolled-back offer to be absent: %v", err)
	}
}
