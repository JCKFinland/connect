package flex_offer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestSubmitCounterOfferSucceeds(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	offer, err := service.SubmitCounterOffer(
		ctx, f.auctionID, f.driverID, 2500,
	)
	if err != nil {
		t.Fatalf("submit FLEX counter-offer: %v", err)
	}

	if offer == nil ||
		offer.OfferType != models.FlexOfferTypeCounterOffer ||
		offer.OfferedAmountCents != 2500 ||
		offer.Status != models.FlexOfferStatusPending ||
		offer.AssignmentID != f.assignmentID {
		t.Fatalf("unexpected counter-offer: %+v", offer)
	}

	stored, err := postgresrepo.NewFlexOfferRepository(f.db).
		GetByID(ctx, offer.ID)
	if err != nil {
		t.Fatalf("reload counter-offer: %v", err)
	}

	if stored.OfferType != models.FlexOfferTypeCounterOffer ||
		stored.OfferedAmountCents != 2500 ||
		stored.Status != models.FlexOfferStatusPending {
		t.Fatalf("unexpected persisted counter-offer: %+v", stored)
	}

	var count int
	err = f.db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM flex_offers
        WHERE id = $1
          AND auction_id = $2
          AND driver_id = $3
          AND offer_type = 'COUNTER_OFFER'
          AND offered_amount = 25.00
          AND status = 'PENDING'
    `, offer.ID, f.auctionID, f.driverID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one persisted counter-offer, got %d", count)
	}
}

func TestSubmitCounterOfferPricingBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		valid  bool
	}{
		{"minimum", 2095, true},
		{"maximum", 2561, true},
		{"below minimum", 2094, false},
		{"above maximum", 2562, false},
		{"unchanged passenger fare", 2328, false},
		{"zero", 0, false},
		{"negative", -100, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupFlexAcceptFixture(t)
			ctx := context.Background()

			service := NewService(Dependencies{
				DB:     f.db,
				Config: f.cfg,
			})

			offer, err := service.SubmitCounterOffer(
				ctx, f.auctionID, f.driverID, tc.amount,
			)

			if tc.valid {
				if err != nil {
					t.Fatalf("valid counter-offer rejected: %v", err)
				}
				if offer == nil ||
					offer.OfferedAmountCents != tc.amount ||
					offer.OfferType != models.FlexOfferTypeCounterOffer {
					t.Fatalf("unexpected valid counter-offer: %+v", offer)
				}
			} else {
				if !errors.Is(err, ErrInvalidCounterOfferAmount) {
					t.Fatalf(
						"expected ErrInvalidCounterOfferAmount, got offer=%+v err=%v",
						offer, err,
					)
				}
				if offer != nil {
					t.Fatalf("invalid amount created offer: %+v", offer)
				}
			}

			var count int
			if err := f.db.QueryRow(ctx, `
                SELECT COUNT(*)
                FROM flex_offers
                WHERE auction_id = $1
                  AND driver_id = $2
            `, f.auctionID, f.driverID).Scan(&count); err != nil {
				t.Fatal(err)
			}

			expected := 0
			if tc.valid {
				expected = 1
			}
			if count != expected {
				t.Fatalf("expected %d offers, got %d", expected, count)
			}
		})
	}
}

func TestSubmitCounterOfferRejectsExistingAcceptFare(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()
	service := NewService(Dependencies{DB: f.db, Config: f.cfg})

	first, err := service.SubmitAcceptFare(ctx, f.auctionID, f.driverID)
	if err != nil {
		t.Fatalf("submit initial accepted fare: %v", err)
	}

	second, err := service.SubmitCounterOffer(
		ctx, f.auctionID, f.driverID, 2500,
	)
	if !errors.Is(err, ErrOfferAlreadyExists) || second != nil {
		t.Fatalf("expected duplicate rejection, got offer=%+v err=%v", second, err)
	}

	assertSingleUnchangedFlexOffer(t, f, first)
}

func TestSubmitCounterOfferRejectsReplacementByAcceptFare(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()
	service := NewService(Dependencies{DB: f.db, Config: f.cfg})

	first, err := service.SubmitCounterOffer(
		ctx, f.auctionID, f.driverID, 2500,
	)
	if err != nil {
		t.Fatalf("submit initial counter-offer: %v", err)
	}

	second, err := service.SubmitAcceptFare(
		ctx, f.auctionID, f.driverID,
	)
	if !errors.Is(err, ErrOfferAlreadyExists) || second != nil {
		t.Fatalf("expected duplicate rejection, got offer=%+v err=%v", second, err)
	}

	assertSingleUnchangedFlexOffer(t, f, first)
}

func assertSingleUnchangedFlexOffer(
	t *testing.T,
	f *flexAcceptFixture,
	original *models.FlexOffer,
) {
	t.Helper()

	if original == nil {
		t.Fatal("original offer is nil")
	}

	var count int
	var persistedID string
	var persistedType string
	var persistedAmount string

	err := f.db.QueryRow(context.Background(), `
        SELECT COUNT(*),
               MIN(id::text),
               MIN(offer_type),
               MIN(offered_amount::text)
        FROM flex_offers
        WHERE auction_id = $1
          AND driver_id = $2
    `, f.auctionID, f.driverID).Scan(
		&count,
		&persistedID,
		&persistedType,
		&persistedAmount,
	)
	if err != nil {
		t.Fatalf("verify original offer: %v", err)
	}

	expectedAmount := "23.28"
	if original.OfferType == models.FlexOfferTypeCounterOffer {
		expectedAmount = "25.00"
	}

	if count != 1 ||
		persistedID != original.ID ||
		persistedType != original.OfferType ||
		persistedAmount != expectedAmount {
		t.Fatalf(
			"original offer changed: count=%d id=%s type=%s amount=%s",
			count, persistedID, persistedType, persistedAmount,
		)
	}
}

func TestSubmitCounterOfferRejectsExpiredAuction(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	_, err := f.db.Exec(ctx, `
        UPDATE flex_auctions
        SET opened_at = NOW() - INTERVAL '61 seconds',
            expires_at = NOW() - INTERVAL '1 second'
        WHERE id = $1
    `, f.auctionID)
	if err != nil {
		t.Fatalf("expire auction: %v", err)
	}

	service := NewService(Dependencies{DB: f.db, Config: f.cfg})
	offer, err := service.SubmitCounterOffer(
		ctx, f.auctionID, f.driverID, 2500,
	)

	if !errors.Is(err, ErrAuctionNotEligible) || offer != nil {
		t.Fatalf("expected expired auction rejection, got offer=%+v err=%v", offer, err)
	}

	assertNoCounterOffers(t, f)
}

func TestSubmitCounterOfferRejectsRevokedDriverDocument(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	tag, err := f.db.Exec(ctx, `
        UPDATE driver_documents
        SET status = 'REVOKED',
            revoked_at = NOW(),
            revoked_by_user_id = verified_by_user_id,
            revocation_reason = 'CONNECT FLEX counter-offer integration test',
            updated_at = NOW()
        WHERE driver_id = (
            SELECT id FROM drivers WHERE user_id = $1
        )
          AND document_type = $2
          AND status = 'VERIFIED'
    `, f.driverID, models.DriverDocumentTypeTaxiDriverLicense)
	if err != nil {
		t.Fatalf("revoke regulatory document: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected one revoked document, got %d", tag.RowsAffected())
	}

	service := NewService(Dependencies{DB: f.db, Config: f.cfg})
	offer, err := service.SubmitCounterOffer(
		ctx, f.auctionID, f.driverID, 2500,
	)

	if !errors.Is(err, ErrDriverNotEligible) || offer != nil {
		t.Fatalf("expected driver eligibility rejection, got offer=%+v err=%v", offer, err)
	}

	assertNoCounterOffers(t, f)
}

func assertNoCounterOffers(t *testing.T, f *flexAcceptFixture) {
	t.Helper()

	var count int
	err := f.db.QueryRow(context.Background(), `
        SELECT COUNT(*)
        FROM flex_offers
        WHERE auction_id = $1
          AND driver_id = $2
    `, f.auctionID, f.driverID).Scan(&count)
	if err != nil {
		t.Fatalf("count FLEX offers: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected zero offers, got %d", count)
	}
}

func TestSubmitCounterOfferConcurrentDuplicate(t *testing.T) {
	f := setupFlexAcceptFixture(t)
	ctx := context.Background()

	service := NewService(Dependencies{
		DB:     f.db,
		Config: f.cfg,
	})

	type result struct {
		offer *models.FlexOffer
		err   error
	}

	const workers = 2

	results := make(chan result, workers)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			offer, err := service.SubmitCounterOffer(
				ctx,
				f.auctionID,
				f.driverID,
				2500,
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
				t.Fatal("successful counter-offer returned nil")
			}

			if r.offer.OfferType != models.FlexOfferTypeCounterOffer ||
				r.offer.OfferedAmountCents != 2500 {
				t.Fatalf("unexpected successful counter-offer: %+v", r.offer)
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
				"unexpected concurrent result: offer=%+v err=%v",
				r.offer,
				r.err,
			)
		}
	}

	if successes != 1 || duplicates != 1 {
		t.Fatalf(
			"expected one success and one duplicate; got success=%d duplicate=%d",
			successes,
			duplicates,
		)
	}

	var count int
	var offerType string
	var amount string

	err := f.db.QueryRow(ctx, `
        SELECT COUNT(*),
               MIN(offer_type),
               MIN(offered_amount::text)
        FROM flex_offers
        WHERE auction_id = $1
          AND driver_id = $2
    `, f.auctionID, f.driverID).Scan(
		&count,
		&offerType,
		&amount,
	)
	if err != nil {
		t.Fatalf("verify concurrent counter-offers: %v", err)
	}

	if count != 1 ||
		offerType != models.FlexOfferTypeCounterOffer ||
		amount != "25.00" {
		t.Fatalf(
			"unexpected persisted state: count=%d type=%s amount=%s",
			count,
			offerType,
			amount,
		)
	}
}
