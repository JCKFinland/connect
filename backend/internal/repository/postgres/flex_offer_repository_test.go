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

	pricingProfileID, pricingVersion, err := createFlexPricingFixture(
		ctx,
		tx,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create FLEX offer-test pricing fixture: %v", err)
	}

	auctionRepo := NewFlexAuctionRepositoryWithDB(tx)
	auction := &models.FlexAuction{
		RideRequestID:       rideRequestID,
		PassengerOfferCents: 1500,
		Currency:            "EUR",
		BaselineCents:       1500,
		MinimumOfferCents:   1350,
		MaximumOfferCents:   1650,
		PricingProfileID:    pricingProfileID,
		PricingVersion:      pricingVersion,
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

	if offer.OfferType != models.FlexOfferTypeAcceptFare {
		t.Fatalf(
			"expected legacy offer type ACCEPT_FARE, got %q",
			offer.OfferType,
		)
	}

	if offer.Status != models.FlexOfferStatusPending ||
		offer.RespondedAt != nil {
		t.Fatalf("offer lifecycle not canonicalized: %+v", offer)
	}

	loaded, err := repo.GetByID(ctx, offer.ID)
	if err != nil {
		t.Fatalf("get flex offer: %v", err)
	}

	if loaded.OfferType != models.FlexOfferTypeAcceptFare {
		t.Fatalf(
			"persisted offer type mismatch: %q",
			loaded.OfferType,
		)
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
	if offers[0].OfferType != models.FlexOfferTypeAcceptFare {
		t.Fatalf(
			"listed offer type mismatch: %q",
			offers[0].OfferType,
		)
	}

	locked, err := repo.GetByIDForUpdate(ctx, offer.ID)
	if err != nil {
		t.Fatalf("lock flex offer: %v", err)
	}
	if locked.ID != offer.ID {
		t.Fatalf("locked unexpected offer: %s", locked.ID)
	}
	if locked.OfferType != models.FlexOfferTypeAcceptFare {
		t.Fatalf(
			"locked offer type mismatch: %q",
			locked.OfferType,
		)
	}

	// Repository validation must reject unsupported classifications.
	invalid := *offer
	invalid.ID = ""
	invalid.OfferType = "UNSUPPORTED"

	if err := repo.Create(ctx, &invalid); err == nil {
		t.Fatal("expected invalid offer type rejection")
	}

	// The database must prevent changing offer classification.
	if _, err := tx.Exec(
		ctx,
		"SAVEPOINT flex_offer_immutable",
	); err != nil {
		t.Fatalf("create immutability savepoint: %v", err)
	}

	_, err = tx.Exec(
		ctx,
		`UPDATE flex_offers
		 SET offer_type = 'COUNTER_OFFER'
		 WHERE id = $1`,
		offer.ID,
	)

	var immutableErr *pgconn.PgError
	if !errors.As(err, &immutableErr) ||
		immutableErr.Code != "23514" {
		t.Fatalf(
			"expected immutable offer type violation 23514, got: %v",
			err,
		)
	}

	if _, err := tx.Exec(
		ctx,
		"ROLLBACK TO SAVEPOINT flex_offer_immutable",
	); err != nil {
		t.Fatalf("rollback immutability savepoint: %v", err)
	}
	if _, err := tx.Exec(
		ctx,
		"RELEASE SAVEPOINT flex_offer_immutable",
	); err != nil {
		t.Fatalf("release immutability savepoint: %v", err)
	}

	// Legitimate lifecycle timestamp updates must remain possible.
	if _, err := tx.Exec(
		ctx,
		`UPDATE flex_offers
		 SET updated_at = updated_at + INTERVAL '1 second'
		 WHERE id = $1`,
		offer.ID,
	); err != nil {
		t.Fatalf("lifecycle timestamp update rejected: %v", err)
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

	// A separate ride request is required because each ride request
	// can have only one FLEX auction.
	counterRideRequestID := uuid.NewString()

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
			'Counter Offer Test Pickup', 60.2055, 24.6559,
			'Counter Offer Test Destination', 60.1719, 24.9414,
			'STANDARD', 1, 'PENDING', $3, $4, $3, $3
		)
		`,
		counterRideRequestID,
		fixture.UserID,
		now,
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("create counter-offer ride request: %v", err)
	}

	// The same driver can offer on a different auction without
	// weakening one-offer-per-driver-per-auction uniqueness.
	counterAuction := &models.FlexAuction{
		RideRequestID:       counterRideRequestID,
		PassengerOfferCents: 1500,
		Currency:            "EUR",
		BaselineCents:       1500,
		MinimumOfferCents:   1350,
		MaximumOfferCents:   1650,
		PricingProfileID:    pricingProfileID,
		PricingVersion:      pricingVersion,
		OpenedAt:            now,
		ExpiresAt:           now.Add(10 * time.Minute),
	}

	if err := auctionRepo.Create(ctx, counterAuction); err != nil {
		t.Fatalf("create counter-offer auction: %v", err)
	}

	counter := &models.FlexOffer{
		AuctionID:          counterAuction.ID,
		AssignmentID:       fixture.AssignmentID,
		DriverID:           fixture.UserID,
		VehicleID:          fixture.VehicleID,
		CompanyID:          fixture.CompanyID,
		BranchID:           fixture.BranchID,
		FleetID:            fixture.FleetID,
		OfferedAmountCents: 1600,
		OfferType:          models.FlexOfferTypeCounterOffer,
		OfferedAt:          now,
		ExpiresAt:          now.Add(5 * time.Minute),
	}

	if err := repo.Create(ctx, counter); err != nil {
		t.Fatalf("create counter-offer: %v", err)
	}

	counterLoaded, err := repo.GetByID(ctx, counter.ID)
	if err != nil {
		t.Fatalf("get counter-offer: %v", err)
	}

	if counterLoaded.OfferType != models.FlexOfferTypeCounterOffer ||
		counterLoaded.OfferedAmountCents != 1600 ||
		counterLoaded.Status != models.FlexOfferStatusPending {
		t.Fatalf(
			"counter-offer persistence mismatch: %+v",
			counterLoaded,
		)
	}

	counterListed, err := repo.ListByAuctionID(
		ctx,
		counterAuction.ID,
	)
	if err != nil {
		t.Fatalf("list counter-offers: %v", err)
	}
	if len(counterListed) != 1 ||
		counterListed[0].ID != counter.ID ||
		counterListed[0].OfferType != models.FlexOfferTypeCounterOffer {
		t.Fatalf(
			"counter-offer list mismatch: %+v",
			counterListed,
		)
	}

	counterLocked, err := repo.GetByIDForUpdate(ctx, counter.ID)
	if err != nil {
		t.Fatalf("lock counter-offer: %v", err)
	}
	if counterLocked.OfferType != models.FlexOfferTypeCounterOffer {
		t.Fatalf(
			"locked counter-offer type mismatch: %q",
			counterLocked.OfferType,
		)
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
