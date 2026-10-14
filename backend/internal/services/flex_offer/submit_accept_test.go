package flex_offer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type flexAcceptFixture struct {
	db           *pgxpool.Pool
	cfg          *config.Config
	driverID     string
	auctionID    string
	rideID       string
	assignmentID string
}

func setupFlexAcceptFixture(t *testing.T) *flexAcceptFixture {
	t.Helper()
	f := testutil.SetupFlexAcceptFixture(t)
	return &flexAcceptFixture{
		db:           f.DB,
		cfg:          f.Config,
		driverID:     f.DriverID,
		auctionID:    f.AuctionID,
		rideID:       f.RideID,
		assignmentID: f.AssignmentID,
	}
}

func TestSubmitAcceptFareSucceeds(t *testing.T) {
	f := setupFlexAcceptFixture(t)

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	offer, err := service.SubmitAcceptFare(
		context.Background(),
		f.auctionID,
		f.driverID,
	)
	if err != nil {
		t.Fatalf("submit FLEX accept fare: %v", err)
	}

	if offer == nil {
		t.Fatal("expected persisted FLEX offer")
	}

	if offer.AuctionID != f.auctionID ||
		offer.DriverID != f.driverID ||
		offer.AssignmentID != f.assignmentID ||
		offer.OfferedAmountCents != 2328 ||
		offer.Status != models.FlexOfferStatusPending {
		t.Fatalf("unexpected FLEX offer: %+v", offer)
	}

	var count int
	err = f.db.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE id = $1
		  AND auction_id = $2
		  AND driver_id = $3
		  AND assignment_id = $4
		  AND offered_amount = 23.28
		  AND status = 'PENDING'
	`, offer.ID, f.auctionID, f.driverID,
		f.assignmentID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("expected one persisted FLEX offer, got %d", count)
	}
}

func TestSubmitAcceptFareRejectsDuplicate(t *testing.T) {
	f := setupFlexAcceptFixture(t)

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	ctx := context.Background()

	first, err := service.SubmitAcceptFare(
		ctx,
		f.auctionID,
		f.driverID,
	)
	if err != nil {
		t.Fatalf("first FLEX offer submission: %v", err)
	}

	if first == nil || first.ID == "" {
		t.Fatal("expected first persisted FLEX offer")
	}

	second, err := service.SubmitAcceptFare(
		ctx,
		f.auctionID,
		f.driverID,
	)

	if !errors.Is(err, ErrOfferAlreadyExists) {
		t.Fatalf(
			"expected ErrOfferAlreadyExists, got offer=%+v err=%v",
			second,
			err,
		)
	}

	if second != nil {
		t.Fatalf("duplicate submission returned an offer: %+v", second)
	}

	var count int
	var persistedID string
	var persistedAmount string

	err = f.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       MIN(id::text),
		       MIN(offered_amount::text)
		FROM flex_offers
		WHERE auction_id = $1
		  AND driver_id = $2
	`, f.auctionID, f.driverID).Scan(
		&count,
		&persistedID,
		&persistedAmount,
	)
	if err != nil {
		t.Fatalf("verify persisted FLEX offer: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected exactly one offer, got %d", count)
	}

	if persistedID != first.ID {
		t.Fatalf(
			"original offer changed: expected %s, got %s",
			first.ID,
			persistedID,
		)
	}

	if persistedAmount != "23.28" {
		t.Fatalf(
			"original fare changed: expected 23.28, got %s",
			persistedAmount,
		)
	}
}

func TestSubmitAcceptFareRejectsExpiredAuction(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	// Expire the auction while retaining its OPEN status.
	// This verifies deadline enforcement independently
	// of any background auction-expiration worker.
	_, err := f.db.Exec(ctx, `
		UPDATE flex_auctions
		SET opened_at = NOW() - INTERVAL '61 seconds',
		    expires_at = NOW() - INTERVAL '1 second'
		WHERE id = $1
	`, f.auctionID)
	if err != nil {
		t.Fatalf("expire FLEX auction: %v", err)
	}

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	offer, err := service.SubmitAcceptFare(
		ctx,
		f.auctionID,
		f.driverID,
	)

	if !errors.Is(err, ErrAuctionNotEligible) {
		t.Fatalf(
			"expected ErrAuctionNotEligible, got offer=%+v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf("expired auction returned offer: %+v", offer)
	}

	var count int
	err = f.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
	`, f.auctionID).Scan(&count)
	if err != nil {
		t.Fatalf("count expired-auction offers: %v", err)
	}

	if count != 0 {
		t.Fatalf(
			"expected zero offers for expired auction, got %d",
			count,
		)
	}
}

func TestSubmitAcceptFareRejectsRevokedDriverDocument(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	// Revoke a required regulatory document while leaving the
	// driver's assignment and online presence unchanged.
	tag, err := f.db.Exec(ctx, `
		UPDATE driver_documents
		SET status = 'REVOKED',
		    revoked_at = NOW(),
		    revoked_by_user_id = verified_by_user_id,
		    revocation_reason = 'CONNECT FLEX regulatory integration test',
		    updated_at = NOW()
		WHERE driver_id = (
			SELECT id
			FROM drivers
			WHERE user_id = $1
		)
		  AND document_type = $2
		  AND status = 'VERIFIED'
	`,
		f.driverID,
		models.DriverDocumentTypeTaxiDriverLicense,
	)
	if err != nil {
		t.Fatalf("revoke driver regulatory document: %v", err)
	}

	if tag.RowsAffected() != 1 {
		t.Fatalf(
			"expected exactly one revoked regulatory document, got %d",
			tag.RowsAffected(),
		)
	}

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	offer, err := service.SubmitAcceptFare(
		ctx,
		f.auctionID,
		f.driverID,
	)

	if !errors.Is(err, ErrDriverNotEligible) {
		t.Fatalf(
			"expected ErrDriverNotEligible, got offer=%+v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf(
			"regulatory-ineligible driver received offer: %+v",
			offer,
		)
	}

	var count int
	err = f.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
		  AND driver_id = $2
	`, f.auctionID, f.driverID).Scan(&count)
	if err != nil {
		t.Fatalf("count regulatory-ineligible driver offers: %v", err)
	}

	if count != 0 {
		t.Fatalf(
			"expected zero offers from regulatory-ineligible driver, got %d",
			count,
		)
	}
}

func TestSubmitAcceptFareConcurrentDuplicate(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	const workers = 2

	type result struct {
		offer *models.FlexOffer
		err   error
	}

	results := make(chan result, workers)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			offer, err := service.SubmitAcceptFare(
				ctx,
				f.auctionID,
				f.driverID,
			)

			results <- result{
				offer: offer,
				err:   err,
			}
		}()
	}

	close(start)

	wg.Wait()
	close(results)

	successes := 0
	duplicates := 0

	for r := range results {
		switch {
		case r.err == nil:
			if r.offer == nil {
				t.Fatal("successful submission returned nil offer")
			}
			successes++

		case errors.Is(r.err, ErrOfferAlreadyExists):
			if r.offer != nil {
				t.Fatalf(
					"duplicate submission returned offer: %+v",
					r.offer,
				)
			}
			duplicates++

		default:
			t.Fatalf(
				"unexpected concurrent submission result: offer=%+v err=%v",
				r.offer,
				r.err,
			)
		}
	}

	if successes != 1 || duplicates != 1 {
		t.Fatalf(
			"expected one success and one duplicate, got successes=%d duplicates=%d",
			successes,
			duplicates,
		)
	}

	var count int

	err := f.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
		  AND driver_id = $2
	`, f.auctionID, f.driverID).Scan(&count)

	if err != nil {
		t.Fatalf("count concurrent FLEX offers: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected exactly one persisted FLEX offer, got %d",
			count,
		)
	}
}

func TestSubmitAcceptFareRejectsDriverAggregateLockContention(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	var permanentDriverID string

	err := f.db.QueryRow(ctx, `
		SELECT id
		FROM drivers
		WHERE user_id = $1
		  AND deleted_at IS NULL
	`, f.driverID).Scan(&permanentDriverID)

	if err != nil {
		t.Fatalf("resolve permanent driver ID: %v", err)
	}

	lockTx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin driver aggregate lock transaction: %v", err)
	}

	lockHeld := true
	t.Cleanup(func() {
		if lockHeld {
			if err := lockTx.Rollback(context.Background()); err != nil {
				t.Errorf("release driver aggregate lock during cleanup: %v", err)
			}
		}
	})

	var lockedID string

	err = lockTx.QueryRow(ctx, `
		SELECT id
		FROM drivers
		WHERE id = $1
		  AND deleted_at IS NULL
		FOR UPDATE
	`, permanentDriverID).Scan(&lockedID)

	if err != nil {
		t.Fatalf("acquire driver aggregate lock: %v", err)
	}

	if lockedID != permanentDriverID {
		t.Fatalf(
			"locked wrong driver: got %s want %s",
			lockedID,
			permanentDriverID,
		)
	}

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	// The driver aggregate lock is held by a separate transaction.
	// FLEX must fail closed rather than creating an offer.
	submitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	offer, err := service.SubmitAcceptFare(
		submitCtx,
		f.auctionID,
		f.driverID,
	)

	if !errors.Is(err, ErrDriverNotEligible) {
		t.Fatalf(
			"expected ErrDriverNotEligible under aggregate lock contention, got offer=%+v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf(
			"driver received FLEX offer while aggregate lock was held: %+v",
			offer,
		)
	}

	var count int

	err = f.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
		  AND driver_id = $2
	`, f.auctionID, f.driverID).Scan(&count)

	if err != nil {
		t.Fatalf("count offers during aggregate lock contention: %v", err)
	}

	if count != 0 {
		t.Fatalf(
			"expected zero offers during driver lock contention, got %d",
			count,
		)
	}

	// Release the lock without modifying the driver's regulatory state.
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release driver aggregate lock: %v", err)
	}
	lockHeld = false

	// Once the lock is released, the still-compliant driver may submit.
	offer, err = service.SubmitAcceptFare(
		ctx,
		f.auctionID,
		f.driverID,
	)

	if err != nil {
		t.Fatalf("submit FLEX offer after lock release: %v", err)
	}

	if offer == nil {
		t.Fatal("expected FLEX offer after aggregate lock release")
	}

	if offer.Status != models.FlexOfferStatusPending {
		t.Fatalf(
			"unexpected offer status after lock release: %s",
			offer.Status,
		)
	}

	err = f.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
		  AND driver_id = $2
	`, f.auctionID, f.driverID).Scan(&count)

	if err != nil {
		t.Fatalf("count offers after aggregate lock release: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected exactly one offer after lock release, got %d",
			count,
		)
	}
}
