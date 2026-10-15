package postgres_test

import (
	"context"
	"errors"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestFlexSelectionRepositoryReservesOfferAndAuction(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	// The shared fixture deletes offers before auctions.
	// A committed selection creates a RESTRICT foreign-key reference
	// from the auction to its selected offer. Release that reference
	// before the fixture's registered cleanup runs.
	t.Cleanup(func() {
		_, err := f.DB.Exec(context.Background(), `
			UPDATE flex_auctions
			SET status = 'OPEN',
			    selected_offer_id = NULL,
			    resolved_at = NULL,
			    updated_at = clock_timestamp()
			WHERE id = $1
			  AND status = 'PENDING_AUTHORIZATION'
		`, f.AuctionID)
		if err != nil {
			t.Errorf("release FLEX selection fixture reference: %v", err)
		}
	})

	service := flex_offer.NewService(flex_offer.Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	offer, err := service.SubmitAcceptFare(ctx, f.AuctionID, f.DriverID)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	tx, err := f.DB.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := postgresrepo.SelectPendingOffer(
		ctx, tx, f.AuctionID, offer.ID,
	); err != nil {
		t.Fatalf("select offer: %v", err)
	}

	if err := postgresrepo.ReserveSelectedOffer(
		ctx, tx, f.AuctionID, offer.ID,
	); err != nil {
		t.Fatalf("reserve auction: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit selection: %v", err)
	}

	var auctionStatus string
	var selectedOfferID string

	err = f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(&auctionStatus, &selectedOfferID)
	if err != nil {
		t.Fatalf("read auction: %v", err)
	}

	if auctionStatus != "PENDING_AUTHORIZATION" {
		t.Fatalf("unexpected auction status: %s", auctionStatus)
	}
	if selectedOfferID != offer.ID {
		t.Fatalf("selected offer mismatch")
	}

	var offerStatus string
	var responded bool

	err = f.DB.QueryRow(ctx, `
		SELECT status, responded_at IS NOT NULL
		FROM flex_offers
		WHERE id = $1
	`, offer.ID).Scan(&offerStatus, &responded)
	if err != nil {
		t.Fatalf("read offer: %v", err)
	}

	if offerStatus != "SELECTED" || !responded {
		t.Fatalf("unexpected selected offer state: %s", offerStatus)
	}
}

func TestFlexSelectionRepositoryRejectsDuplicateSelection(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := flex_offer.NewService(flex_offer.Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	offer, err := service.SubmitAcceptFare(ctx, f.AuctionID, f.DriverID)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	tx, err := f.DB.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := postgresrepo.SelectPendingOffer(
		ctx, tx, f.AuctionID, offer.ID,
	); err != nil {
		t.Fatalf("first selection: %v", err)
	}

	err = postgresrepo.SelectPendingOffer(
		ctx, tx, f.AuctionID, offer.ID,
	)
	if !errors.Is(err, postgresrepo.ErrFlexSelectionConflict) {
		t.Fatalf("expected selection conflict, got: %v", err)
	}
}

func TestFlexSelectionRepositoryRollbackPreservesPendingState(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := flex_offer.NewService(flex_offer.Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	offer, err := service.SubmitAcceptFare(ctx, f.AuctionID, f.DriverID)
	if err != nil {
		t.Fatalf("submit FLEX offer: %v", err)
	}

	tx, err := f.DB.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	if err := postgresrepo.SelectPendingOffer(
		ctx, tx, f.AuctionID, offer.ID,
	); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("select offer: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback selection: %v", err)
	}

	var status string
	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_offers
		WHERE id = $1
	`, offer.ID).Scan(&status)
	if err != nil {
		t.Fatalf("read offer after rollback: %v", err)
	}

	if status != "PENDING" {
		t.Fatalf("rollback failed: offer status = %s", status)
	}

	var auctionStatus string
	var selectedOfferID *string

	err = f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(&auctionStatus, &selectedOfferID)
	if err != nil {
		t.Fatalf("read auction after rollback: %v", err)
	}

	if auctionStatus != "OPEN" || selectedOfferID != nil {
		t.Fatalf("rollback failed: auction status = %s", auctionStatus)
	}
}

var _ pgx.Tx
