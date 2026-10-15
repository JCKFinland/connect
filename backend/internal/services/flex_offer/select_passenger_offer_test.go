package flex_offer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func setupPassengerSelectionTest(
	t *testing.T,
) (*testutil.FlexAcceptFixture, *Service, string, string) {
	t.Helper()

	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.DB,
		Config: f.Config,
	})

	offer, err := service.SubmitAcceptFare(
		ctx,
		f.AuctionID,
		f.DriverID,
	)
	if err != nil {
		t.Fatalf("create FLEX driver offer: %v", err)
	}

	var passengerID string
	err = f.DB.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, f.RideID).Scan(&passengerID)
	if err != nil {
		t.Fatalf("resolve FLEX passenger: %v", err)
	}

	// The shared fixture deletes offers before auctions.
	// A successful selection creates a restrictive reference
	// from the auction to the selected offer.
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
			t.Errorf("release selected FLEX offer reference: %v", err)
		}
	})

	return f, service, passengerID, offer.ID
}

func TestSelectPassengerOfferSuccess(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	ctx := context.Background()

	selected, err := service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offerID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("select passenger offer: %v", err)
	}

	if selected == nil ||
		selected.ID != offerID ||
		selected.Status != models.FlexOfferStatusSelected {
		t.Fatalf("unexpected selected offer: %+v", selected)
	}

	var auctionStatus string
	var selectedOfferID string

	err = f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&auctionStatus,
		&selectedOfferID,
	)
	if err != nil {
		t.Fatalf("read selected FLEX auction: %v", err)
	}

	if auctionStatus != models.FlexAuctionStatusPendingAuthorization {
		t.Fatalf("unexpected auction status: %s", auctionStatus)
	}

	if selectedOfferID != offerID {
		t.Fatalf("auction selected wrong offer")
	}

	var offerStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_offers
		WHERE id = $1
	`, offerID).Scan(&offerStatus)
	if err != nil {
		t.Fatalf("read selected FLEX offer: %v", err)
	}

	if offerStatus != models.FlexOfferStatusSelected {
		t.Fatalf("unexpected persisted offer status: %s", offerStatus)
	}
}

func TestSelectPassengerOfferRejectsNonOwner(t *testing.T) {
	f, service, _, offerID :=
		setupPassengerSelectionTest(t)

	_, err := service.SelectPassengerOffer(
		context.Background(),
		f.RideID,
		f.AuctionID,
		offerID,
		uuid.NewString(),
	)

	if !errors.Is(err, ErrPassengerSelectionAccessDenied) {
		t.Fatalf("expected passenger access denial, got: %v", err)
	}

	assertPassengerSelectionUnchanged(t, f, offerID)
}

func TestSelectPassengerOfferRejectsUnknownOffer(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	_, err := service.SelectPassengerOffer(
		context.Background(),
		f.RideID,
		f.AuctionID,
		uuid.NewString(),
		passengerID,
	)

	if !errors.Is(err, ErrPassengerSelectionNotEligible) {
		t.Fatalf("expected ineligible offer, got: %v", err)
	}

	assertPassengerSelectionUnchanged(t, f, offerID)
}

func TestSelectPassengerOfferRejectsDuplicateSelection(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	ctx := context.Background()

	_, err := service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offerID,
		passengerID,
	)
	if err != nil {
		t.Fatalf("first selection failed: %v", err)
	}

	_, err = service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offerID,
		passengerID,
	)

	if !errors.Is(err, ErrPassengerSelectionNotEligible) {
		t.Fatalf("expected duplicate selection rejection, got: %v", err)
	}
}

func TestSelectPassengerOfferRejectsExpiredOffer(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	ctx := context.Background()

	_, err := f.DB.Exec(ctx, `
		UPDATE flex_offers
		SET offered_at = clock_timestamp() - INTERVAL '2 minutes',
		    expires_at = clock_timestamp() - INTERVAL '1 second',
		    updated_at = clock_timestamp()
		WHERE id = $1
	`, offerID)
	if err != nil {
		t.Fatalf("expire FLEX offer: %v", err)
	}

	_, err = service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offerID,
		passengerID,
	)

	if !errors.Is(err, ErrPassengerSelectionNotEligible) {
		t.Fatalf("expected expired offer rejection, got: %v", err)
	}

	assertPassengerSelectionUnchanged(t, f, offerID)
}

func assertPassengerSelectionUnchanged(
	t *testing.T,
	f *testutil.FlexAcceptFixture,
	offerID string,
) {
	t.Helper()

	ctx := context.Background()

	var auctionStatus string
	var selectedOfferID *string

	err := f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&auctionStatus,
		&selectedOfferID,
	)
	if err != nil {
		t.Fatalf("read FLEX auction: %v", err)
	}

	if auctionStatus != models.FlexAuctionStatusOpen ||
		selectedOfferID != nil {
		t.Fatalf(
			"auction unexpectedly modified: status=%s selected=%v",
			auctionStatus,
			selectedOfferID,
		)
	}

	var offerStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_offers
		WHERE id = $1
	`, offerID).Scan(&offerStatus)
	if err != nil {
		t.Fatalf("read FLEX offer: %v", err)
	}

	if offerStatus != models.FlexOfferStatusPending {
		t.Fatalf("offer unexpectedly modified: %s", offerStatus)
	}
}

func TestSelectPassengerOfferRejectsStaleDriverHeartbeat(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	ctx := context.Background()

	_, err := f.DB.Exec(ctx, `
		UPDATE driver_presence
		SET last_heartbeat_at =
			clock_timestamp() - INTERVAL '24 hours',
		    updated_at = clock_timestamp()
		WHERE driver_id = $1
	`, f.DriverID)
	if err != nil {
		t.Fatalf("make FLEX driver heartbeat stale: %v", err)
	}

	_, err = service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offerID,
		passengerID,
	)

	if !errors.Is(err, ErrPassengerSelectionNotEligible) {
		t.Fatalf(
			"expected stale driver rejection, got: %v",
			err,
		)
	}

	assertPassengerSelectionUnchanged(t, f, offerID)
}

func TestSelectPassengerOfferRejectsChangedAssignment(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	ctx := context.Background()

	// The offer remains pinned to the original assignment.
	// Making that assignment inactive must invalidate selection.
	_, err := f.DB.Exec(ctx, `
		UPDATE driver_assignments
		SET unassigned_at = clock_timestamp(),
		    updated_at = clock_timestamp()
		WHERE id = $1
	`, f.AssignmentID)
	if err != nil {
		t.Fatalf("deactivate FLEX driver assignment: %v", err)
	}

	// Restore the shared driver's original fixture state before
	// its registered cleanup callbacks execute.
	t.Cleanup(func() {
		_, err := f.DB.Exec(context.Background(), `
			UPDATE driver_assignments
			SET unassigned_at = NULL,
			    updated_at = clock_timestamp()
			WHERE id = $1
		`, f.AssignmentID)
		if err != nil {
			t.Errorf("restore FLEX driver assignment: %v", err)
		}
	})

	_, err = service.SelectPassengerOffer(
		ctx,
		f.RideID,
		f.AuctionID,
		offerID,
		passengerID,
	)

	if !errors.Is(err, ErrPassengerSelectionNotEligible) {
		t.Fatalf(
			"expected changed assignment rejection, got: %v",
			err,
		)
	}

	assertPassengerSelectionUnchanged(t, f, offerID)
}

func TestSelectPassengerOfferConcurrentRequestsSingleWinner(t *testing.T) {
	f, service, passengerID, offerID :=
		setupPassengerSelectionTest(t)

	ctx := context.Background()

	const attempts = 2

	start := make(chan struct{})
	results := make(chan error, attempts)

	var wg sync.WaitGroup
	wg.Add(attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()

			<-start

			_, err := service.SelectPassengerOffer(
				ctx,
				f.RideID,
				f.AuctionID,
				offerID,
				passengerID,
			)

			results <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0

	for err := range results {
		switch {
		case err == nil:
			successes++

		case errors.Is(err, ErrPassengerSelectionNotEligible):
			conflicts++

		default:
			t.Fatalf("unexpected concurrent selection error: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf(
			"expected one success and one conflict; successes=%d conflicts=%d",
			successes,
			conflicts,
		)
	}

	var auctionStatus string
	var selectedOfferID string

	err := f.DB.QueryRow(ctx, `
		SELECT status, selected_offer_id
		FROM flex_auctions
		WHERE id = $1
	`, f.AuctionID).Scan(
		&auctionStatus,
		&selectedOfferID,
	)
	if err != nil {
		t.Fatalf("read auction after concurrent selection: %v", err)
	}

	if auctionStatus != models.FlexAuctionStatusPendingAuthorization {
		t.Fatalf("unexpected auction status: %s", auctionStatus)
	}

	if selectedOfferID != offerID {
		t.Fatalf(
			"unexpected selected offer: got=%s want=%s",
			selectedOfferID,
			offerID,
		)
	}

	var selectedCount int

	err = f.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
		  AND status = 'SELECTED'
	`, f.AuctionID).Scan(&selectedCount)
	if err != nil {
		t.Fatalf("count selected FLEX offers: %v", err)
	}

	if selectedCount != 1 {
		t.Fatalf(
			"expected exactly one selected offer, got %d",
			selectedCount,
		)
	}
}
