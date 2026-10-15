package flex_offer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func flexDiscoveryFixture(t *testing.T) (
	*testutil.FlexAcceptFixture,
	*Service,
	string,
) {
	t.Helper()

	f := testutil.SetupFlexAcceptFixture(t)

	var customerID string

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT customer_id FROM ride_requests WHERE id = $1`,
		f.RideID,
	).Scan(&customerID)
	if err != nil {
		t.Fatalf("resolve fixture passenger: %v", err)
	}

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	return f, service, customerID
}

func TestListPassengerOffersSucceeds(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	created, err := service.SubmitCounterOffer(
		ctx,
		f.AuctionID,
		f.DriverID,
		2500,
	)
	if err != nil {
		t.Fatalf("submit counter-offer: %v", err)
	}

	offers, err := service.ListPassengerOffers(
		ctx,
		f.RideID,
		f.AuctionID,
		customerID,
	)
	if err != nil {
		t.Fatalf("list passenger offers: %v", err)
	}

	if len(offers) != 1 {
		t.Fatalf("expected one visible offer, got %d", len(offers))
	}

	got := offers[0]

	if got.ID != created.ID ||
		got.AuctionID != f.AuctionID ||
		got.OfferType != models.FlexOfferTypeCounterOffer ||
		got.OfferedAmountCents != 2500 ||
		got.Status != models.FlexOfferStatusPending {
		t.Fatalf("unexpected passenger-visible offer: %+v", got)
	}
}

func TestListPassengerOffersRejectsNonOwner(t *testing.T) {
	f, service, _ := flexDiscoveryFixture(t)
	ctx := context.Background()

	_, err := service.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit driver offer: %v", err)
	}

	offers, err := service.ListPassengerOffers(
		ctx,
		f.RideID,
		f.AuctionID,
		f.DriverID,
	)

	if !errors.Is(err, ErrPassengerOfferAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}

	if offers != nil {
		t.Fatalf("unauthorized caller received offers: %+v", offers)
	}
}

func TestListPassengerOffersRejectsWrongAuction(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	offers, err := service.ListPassengerOffers(
		ctx,
		f.RideID,
		uuid.NewString(),
		customerID,
	)

	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected auction not found, got %v", err)
	}

	if offers != nil {
		t.Fatalf("unexpected offers for wrong auction: %+v", offers)
	}
}

func TestListPassengerOffersRejectsExpiredAuction(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	_, err := service.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit driver offer: %v", err)
	}

	now := time.Now().UTC()

	_, err = f.DB.Exec(
		ctx,
		`UPDATE flex_auctions
		 SET opened_at = $2,
		     expires_at = $3
		 WHERE id = $1`,
		f.AuctionID,
		now.Add(-2*time.Minute),
		now.Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("expire auction: %v", err)
	}

	offers, err := service.ListPassengerOffers(
		ctx,
		f.RideID,
		f.AuctionID,
		customerID,
	)

	if !errors.Is(err, ErrPassengerOfferAuctionNotEligible) {
		t.Fatalf("expected auction not eligible, got %v", err)
	}

	if offers != nil {
		t.Fatalf("expired auction exposed offers: %+v", offers)
	}
}

func TestListPassengerOffersExcludesExpiredOffer(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit driver offer: %v", err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_offers
		SET offered_at = NOW() - INTERVAL '2 minutes',
		    expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, offer.ID)
	if err != nil {
		t.Fatalf("expire offer: %v", err)
	}

	offers, err := service.ListPassengerOffers(
		ctx, f.RideID, f.AuctionID, customerID,
	)
	if err != nil {
		t.Fatalf("discover offers: %v", err)
	}

	if len(offers) != 0 {
		t.Fatalf("expired offer was exposed: %+v", offers)
	}
}

func TestListPassengerOffersExcludesNonPendingOffer(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	offer, err := service.SubmitAcceptFare(
		ctx, f.AuctionID, f.DriverID,
	)
	if err != nil {
		t.Fatalf("submit driver offer: %v", err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_offers
		SET status = 'REJECTED',
		    responded_at = NOW()
		WHERE id = $1
	`, offer.ID)
	if err != nil {
		t.Fatalf("reject offer: %v", err)
	}

	offers, err := service.ListPassengerOffers(
		ctx, f.RideID, f.AuctionID, customerID,
	)
	if err != nil {
		t.Fatalf("discover offers: %v", err)
	}

	if len(offers) != 0 {
		t.Fatalf("non-pending offer was exposed: %+v", offers)
	}
}

func TestListPassengerOffersRejectsExpiredRideRequest(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	_, err := f.DB.Exec(ctx, `
		UPDATE ride_requests
		SET expires_at = NOW() - INTERVAL '1 second'
		WHERE id = $1
	`, f.RideID)
	if err != nil {
		t.Fatalf("expire ride request: %v", err)
	}

	offers, err := service.ListPassengerOffers(
		ctx, f.RideID, f.AuctionID, customerID,
	)

	if !errors.Is(err, ErrPassengerOfferAuctionNotEligible) {
		t.Fatalf("expected ineligible request, got %v", err)
	}

	if offers != nil {
		t.Fatalf("expired request exposed offers: %+v", offers)
	}
}

func TestListPassengerOffersDoesNotMutatePersistence(t *testing.T) {
	f, service, customerID := flexDiscoveryFixture(t)
	ctx := context.Background()

	offer, err := service.SubmitCounterOffer(
		ctx, f.AuctionID, f.DriverID, 2500,
	)
	if err != nil {
		t.Fatalf("submit counter-offer: %v", err)
	}

	var beforeOfferStatus, beforeAuctionStatus, beforeRideStatus string
	var beforeOfferUpdatedAt, beforeAuctionUpdatedAt, beforeRideUpdatedAt time.Time
	var beforeSelectedOfferID *string

	err = f.DB.QueryRow(ctx, `
		SELECT status, updated_at
		FROM flex_offers
		WHERE id = $1
	`, offer.ID).Scan(&beforeOfferStatus, &beforeOfferUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id, updated_at
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&beforeAuctionStatus,
		&beforeSelectedOfferID,
		&beforeAuctionUpdatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT status, updated_at
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&beforeRideStatus, &beforeRideUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}

	offers, err := service.ListPassengerOffers(
		ctx, f.RideID, f.AuctionID, customerID,
	)
	if err != nil {
		t.Fatalf("discover offers: %v", err)
	}
	if len(offers) != 1 {
		t.Fatalf("expected one offer, got %d", len(offers))
	}

	var afterOfferStatus, afterAuctionStatus, afterRideStatus string
	var afterOfferUpdatedAt, afterAuctionUpdatedAt, afterRideUpdatedAt time.Time
	var afterSelectedOfferID *string

	err = f.DB.QueryRow(ctx, `
		SELECT status, updated_at
		FROM flex_offers
		WHERE id = $1
	`, offer.ID).Scan(&afterOfferStatus, &afterOfferUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id, updated_at
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&afterAuctionStatus,
		&afterSelectedOfferID,
		&afterAuctionUpdatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = f.DB.QueryRow(ctx, `
		SELECT status, updated_at
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&afterRideStatus, &afterRideUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}

	if beforeOfferStatus != afterOfferStatus ||
		!beforeOfferUpdatedAt.Equal(afterOfferUpdatedAt) ||
		beforeAuctionStatus != afterAuctionStatus ||
		!beforeAuctionUpdatedAt.Equal(afterAuctionUpdatedAt) ||
		beforeRideStatus != afterRideStatus ||
		!beforeRideUpdatedAt.Equal(afterRideUpdatedAt) {
		t.Fatal("discovery unexpectedly mutated persisted lifecycle state")
	}

	if beforeSelectedOfferID != nil || afterSelectedOfferID != nil {
		t.Fatal("discovery unexpectedly selected an offer")
	}
}
