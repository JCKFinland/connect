package ride_request

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	"github.com/JCKFinland/connect/backend/internal/services/flex_pricing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type flexAuctionFareEstimatorStub struct {
	result *fare_estimate.EstimateResult
	err    error
}

func (s *flexAuctionFareEstimatorStub) Estimate(
	_ context.Context,
	_ fare_estimate.EstimateRequest,
) (*fare_estimate.EstimateResult, error) {
	return s.result, s.err
}

func insertFlexAuctionTestRide(
	t *testing.T,
	db *pgxpool.Pool,
	customerID string,
	status string,
	matchingMode string,
	expiresAt time.Time,
) string {
	t.Helper()

	ctx := context.Background()

	var categoryID string
	if err := db.QueryRow(ctx, `
		SELECT id
		FROM service_categories
		WHERE is_active = TRUE
		ORDER BY id
		LIMIT 1
	`).Scan(&categoryID); err != nil {
		t.Fatalf("resolve active service category: %v", err)
	}

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err := db.Exec(ctx, `
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
			service_category_id,
			passenger_count,
			status,
			matching_mode,
			notes,
			requested_at,
			expires_at,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2,
			'FLEX Auction Test Pickup',
			60.2055, 24.6559,
			'FLEX Auction Test Destination',
			60.1719, 24.9414,
			'STANDARD',
			$3,
			1,
			$4,
			$5,
			'FLEX auction integration test',
			$6,
			$7,
			$6,
			$6
		)
	`,
		rideID,
		customerID,
		categoryID,
		status,
		matchingMode,
		now,
		expiresAt,
	)
	if err != nil {
		t.Fatalf("insert FLEX auction test ride: %v", err)
	}

	t.Cleanup(func() {
		// Auction rows reference ride requests with ON DELETE RESTRICT.
		// Delete dependent test auctions before deleting the test ride.
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM flex_auctions WHERE ride_request_id = $1`,
			rideID,
		); err != nil {
			t.Errorf("cleanup FLEX auction: %v", err)
		}

		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		); err != nil {
			t.Errorf("cleanup FLEX auction test ride: %v", err)
		}
	})

	return rideID
}

func TestInitiateFlexAuctionAuthorizedSucceeds(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	pricingProfileID, pricingVersion := createFlexServicePricingFixture(t, db)

	rideID := insertFlexAuctionTestRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeFlex,
		time.Now().UTC().Add(time.Hour),
	)

	estimator := &flexAuctionFareEstimatorStub{
		result: &fare_estimate.EstimateResult{
			TotalAmount:      23.28,
			Currency:         "EUR",
			PricingProfileID: pricingProfileID,
			PricingVersion:   pricingVersion,
		},
	}

	service := NewService(Dependencies{
		DB:            db,
		UserRoles:     repository.NewUserRoleRepository(db),
		FareEstimator: estimator,
	})

	before := time.Now().UTC()

	result, err := service.InitiateFlexAuctionAuthorized(
		ctx,
		rideID,
		customerID,
		2328,
	)
	if err != nil {
		t.Fatalf("initiate FLEX auction: %v", err)
	}
	if result == nil || result.Auction == nil {
		t.Fatal("expected persisted FLEX auction")
	}

	auction := result.Auction

	if auction.RideRequestID != rideID {
		t.Fatalf("unexpected ride request ID: %s", auction.RideRequestID)
	}
	if auction.PassengerOfferCents != 2328 {
		t.Fatalf("unexpected passenger offer: %d", auction.PassengerOfferCents)
	}
	if auction.Currency != "EUR" {
		t.Fatalf("unexpected currency: %s", auction.Currency)
	}
	if auction.Status != models.FlexAuctionStatusOpen {
		t.Fatalf("unexpected auction status: %s", auction.Status)
	}
	if auction.ExpiresAt.Sub(auction.OpenedAt) > 60*time.Second {
		t.Fatalf("auction lifetime exceeds 60 seconds")
	}
	if auction.OpenedAt.Before(before) {
		t.Fatalf("auction opened before initiation")
	}

	if result.PricingBounds.BaselineCents != 2328 ||
		result.PricingBounds.MinimumCents != 2095 ||
		result.PricingBounds.MaximumCents != 2561 {
		t.Fatalf("unexpected pricing bounds: %+v", result.PricingBounds)
	}

	if auction.BaselineCents != 2328 ||
		auction.MinimumOfferCents != 2095 ||
		auction.MaximumOfferCents != 2561 ||
		auction.PricingProfileID != pricingProfileID ||
		auction.PricingVersion != pricingVersion {
		t.Fatalf("returned auction pricing provenance mismatch: %+v", auction)
	}

	var persistedBaseline string
	var persistedMinimum string
	var persistedMaximum string
	var persistedProfileID string
	var persistedVersion string

	var persistedStatus string
	var persistedOffer string
	var persistedCurrency string
	var persistedExpiry time.Time

	err = db.QueryRow(ctx, `
		SELECT
			status,
			passenger_offer_amount::text,
			currency,
			expires_at,
			baseline_amount::text,
			minimum_offer_amount::text,
			maximum_offer_amount::text,
			pricing_profile_id,
			pricing_version
		FROM flex_auctions
		WHERE ride_request_id = $1
	`, rideID).Scan(
		&persistedStatus,
		&persistedOffer,
		&persistedCurrency,
		&persistedExpiry,
		&persistedBaseline,
		&persistedMinimum,
		&persistedMaximum,
		&persistedProfileID,
		&persistedVersion,
	)
	if err != nil {
		t.Fatalf("load persisted FLEX auction: %v", err)
	}

	if persistedStatus != models.FlexAuctionStatusOpen ||
		persistedOffer != "23.28" ||
		persistedCurrency != "EUR" {
		t.Fatalf(
			"unexpected persisted auction: status=%s offer=%s currency=%s",
			persistedStatus,
			persistedOffer,
			persistedCurrency,
		)
	}

	if persistedBaseline != "23.28" ||
		persistedMinimum != "20.95" ||
		persistedMaximum != "25.61" ||
		persistedProfileID != pricingProfileID ||
		persistedVersion != pricingVersion {
		t.Fatalf(
			"persisted FLEX pricing provenance mismatch: baseline=%s minimum=%s maximum=%s profile=%s version=%s",
			persistedBaseline,
			persistedMinimum,
			persistedMaximum,
			persistedProfileID,
			persistedVersion,
		)
	}

	if !persistedExpiry.Equal(auction.ExpiresAt) {
		t.Fatalf("persisted auction expiry differs from returned expiry")
	}

	var count int
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_auctions
		WHERE ride_request_id = $1
	`, rideID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one persisted auction, got %d", count)
	}
}

func TestInitiateFlexAuctionAuthorizedRejectsNonOwner(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexAuctionTestRide(
		t, db, customerID,
		"PENDING",
		models.RideMatchingModeFlex,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
		FareEstimator: &flexAuctionFareEstimatorStub{
			result: &fare_estimate.EstimateResult{
				TotalAmount: 23.28,
				Currency:    "EUR",
			},
		},
	})

	result, err := service.InitiateFlexAuctionAuthorized(
		ctx,
		rideID,
		uuid.NewString(),
		2328,
	)

	if !errors.Is(err, ErrFlexAuctionAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}
	if result != nil {
		t.Fatal("unauthorized initiation returned an auction")
	}

	assertFlexAuctionCount(t, db, rideID, 0)
}

func TestInitiateFlexAuctionAuthorizedRejectsOwnerWithoutCustomerRole(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexAuctionTestRide(
		t, db, customerID,
		"PENDING",
		models.RideMatchingModeFlex,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB: db,
		UserRoles: &createUserRoleTestRepository{
			roles: []string{"DRIVER"},
		},
		FareEstimator: &flexAuctionFareEstimatorStub{
			result: &fare_estimate.EstimateResult{
				TotalAmount: 23.28,
				Currency:    "EUR",
			},
		},
	})

	result, err := service.InitiateFlexAuctionAuthorized(
		ctx,
		rideID,
		customerID,
		2328,
	)

	if !errors.Is(err, ErrFlexAuctionAccessDenied) {
		t.Fatalf("expected CUSTOMER role access denied, got %v", err)
	}
	if result != nil {
		t.Fatal("unauthorized initiation returned an auction")
	}

	assertFlexAuctionCount(t, db, rideID, 0)
}

func TestInitiateFlexAuctionAuthorizedRejectsDuplicate(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	pricingProfileID, pricingVersion := createFlexServicePricingFixture(t, db)

	rideID := insertFlexAuctionTestRide(
		t, db, customerID,
		"PENDING",
		models.RideMatchingModeFlex,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
		FareEstimator: &flexAuctionFareEstimatorStub{
			result: &fare_estimate.EstimateResult{
				TotalAmount:      23.28,
				Currency:         "EUR",
				PricingProfileID: pricingProfileID,
				PricingVersion:   pricingVersion,
			},
		},
	})

	first, err := service.InitiateFlexAuctionAuthorized(
		ctx, rideID, customerID, 2328,
	)
	if err != nil {
		t.Fatalf("first initiation: %v", err)
	}

	second, err := service.InitiateFlexAuctionAuthorized(
		ctx, rideID, customerID, 2328,
	)

	if !errors.Is(err, ErrFlexAuctionExists) {
		t.Fatalf("expected duplicate auction error, got %v", err)
	}
	if second != nil {
		t.Fatal("duplicate initiation returned an auction")
	}

	assertFlexAuctionCount(t, db, rideID, 1)

	var persistedID string
	if err := db.QueryRow(ctx, `
		SELECT id
		FROM flex_auctions
		WHERE ride_request_id = $1
	`, rideID).Scan(&persistedID); err != nil {
		t.Fatal(err)
	}

	if persistedID != first.Auction.ID {
		t.Fatalf("duplicate attempt replaced the original auction")
	}
}

func assertFlexAuctionCount(
	t *testing.T,
	db *pgxpool.Pool,
	rideID string,
	expected int,
) {
	t.Helper()

	var count int
	if err := db.QueryRow(
		context.Background(),
		`SELECT COUNT(*) FROM flex_auctions WHERE ride_request_id = $1`,
		rideID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != expected {
		t.Fatalf(
			"expected %d FLEX auctions, got %d",
			expected,
			count,
		)
	}
}

// TestInitiateFlexAuctionAuthorizedConcurrentAttempts verifies that
// concurrent customer requests cannot create duplicate FLEX auctions.
func TestInitiateFlexAuctionAuthorizedConcurrentAttempts(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	pricingProfileID, pricingVersion := createFlexServicePricingFixture(t, db)

	rideID := insertFlexAuctionTestRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeFlex,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
		FareEstimator: &flexAuctionFareEstimatorStub{
			result: &fare_estimate.EstimateResult{
				TotalAmount:      23.28,
				Currency:         "EUR",
				PricingProfileID: pricingProfileID,
				PricingVersion:   pricingVersion,
			},
		},
	})

	const attempts = 2

	type initiationResult struct {
		result *InitiateFlexAuctionResult
		err    error
	}

	start := make(chan struct{})
	results := make(chan initiationResult, attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			<-start

			callCtx, cancel := context.WithTimeout(
				ctx,
				10*time.Second,
			)
			defer cancel()

			result, err := service.InitiateFlexAuctionAuthorized(
				callCtx,
				rideID,
				customerID,
				2328,
			)

			results <- initiationResult{
				result: result,
				err:    err,
			}
		}()
	}

	close(start)

	successes := 0
	duplicates := 0

	for i := 0; i < attempts; i++ {
		select {
		case outcome := <-results:
			switch {
			case outcome.err == nil:
				if outcome.result == nil ||
					outcome.result.Auction == nil {
					t.Fatal("successful initiation returned nil auction")
				}
				successes++

			case errors.Is(outcome.err, ErrFlexAuctionExists):
				if outcome.result != nil {
					t.Fatal("duplicate initiation returned an auction")
				}
				duplicates++

			default:
				t.Fatalf(
					"unexpected concurrent initiation error: %v",
					outcome.err,
				)
			}

		case <-time.After(15 * time.Second):
			t.Fatal("concurrent FLEX auction initiation timed out")
		}
	}

	if successes != 1 {
		t.Fatalf(
			"expected exactly one successful initiation, got %d",
			successes,
		)
	}

	if duplicates != 1 {
		t.Fatalf(
			"expected exactly one duplicate rejection, got %d",
			duplicates,
		)
	}

	assertFlexAuctionCount(t, db, rideID, 1)
}

// TestInitiateFlexAuctionAuthorizedRejectsIneligibleLifecycleStates
// verifies that only unexpired PENDING/FLEX requests can initiate auctions.
func TestInitiateFlexAuctionAuthorizedRejectsIneligibleLifecycleStates(
	t *testing.T,
) {
	db, customerID := setupFlexActivationTest(t)

	tests := []struct {
		name         string
		status       string
		matchingMode string
		expiresAt    time.Time
	}{
		{
			name:         "STANDARD request",
			status:       "PENDING",
			matchingMode: models.RideMatchingModeStandard,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "cancelled FLEX request",
			status:       "CANCELLED",
			matchingMode: models.RideMatchingModeFlex,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "matching FLEX request",
			status:       "MATCHING",
			matchingMode: models.RideMatchingModeFlex,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "accepted FLEX request",
			status:       "ACCEPTED",
			matchingMode: models.RideMatchingModeFlex,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "expired FLEX request",
			status:       "PENDING",
			matchingMode: models.RideMatchingModeFlex,
			expiresAt:    time.Now().UTC().Add(-time.Minute),
		},
	}

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
		FareEstimator: &flexAuctionFareEstimatorStub{
			result: &fare_estimate.EstimateResult{
				TotalAmount: 23.28,
				Currency:    "EUR",
			},
		},
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rideID := insertFlexAuctionTestRide(
				t,
				db,
				customerID,
				tc.status,
				tc.matchingMode,
				tc.expiresAt,
			)

			result, err := service.InitiateFlexAuctionAuthorized(
				context.Background(),
				rideID,
				customerID,
				2328,
			)

			if !errors.Is(err, ErrFlexAuctionNotEligible) {
				t.Fatalf(
					"expected ineligible FLEX request, got %v",
					err,
				)
			}
			if result != nil {
				t.Fatal("ineligible request returned an auction")
			}

			assertFlexAuctionCount(t, db, rideID, 0)
		})
	}
}

// TestInitiateFlexAuctionAuthorizedRejectsInvalidOffers verifies
// that invalid passenger bids never create persisted auctions.
func TestInitiateFlexAuctionAuthorizedRejectsInvalidOffers(
	t *testing.T,
) {
	db, customerID := setupFlexActivationTest(t)

	// Canonical estimate: EUR 23.28.
	// Permitted range: 2095 through 2561 cents.
	tests := []struct {
		name       string
		offerCents int64
		expected   error
	}{
		{
			name:       "zero offer",
			offerCents: 0,
			expected:   flex_pricing.ErrInvalidOffer,
		},
		{
			name:       "negative offer",
			offerCents: -1,
			expected:   flex_pricing.ErrInvalidOffer,
		},
		{
			name:       "below minimum",
			offerCents: 2094,
			expected:   flex_pricing.ErrOfferOutOfRange,
		},
		{
			name:       "above maximum",
			offerCents: 2562,
			expected:   flex_pricing.ErrOfferOutOfRange,
		},
	}

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
		FareEstimator: &flexAuctionFareEstimatorStub{
			result: &fare_estimate.EstimateResult{
				TotalAmount: 23.28,
				Currency:    "EUR",
			},
		},
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rideID := insertFlexAuctionTestRide(
				t,
				db,
				customerID,
				"PENDING",
				models.RideMatchingModeFlex,
				time.Now().UTC().Add(time.Hour),
			)

			result, err := service.InitiateFlexAuctionAuthorized(
				context.Background(),
				rideID,
				customerID,
				tc.offerCents,
			)

			if !errors.Is(err, tc.expected) {
				t.Fatalf(
					"expected %v, got %v",
					tc.expected,
					err,
				)
			}
			if result != nil {
				t.Fatal("invalid offer returned an auction")
			}

			assertFlexAuctionCount(t, db, rideID, 0)
		})
	}
}

// flexAuctionBlockingEstimator allows a test to change the ride request
// after estimation starts but before auction initiation acquires its locks.
type flexAuctionBlockingEstimator struct {
	started chan struct{}
	release chan struct{}
}

func (s *flexAuctionBlockingEstimator) Estimate(
	ctx context.Context,
	_ fare_estimate.EstimateRequest,
) (*fare_estimate.EstimateResult, error) {
	close(s.started)

	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &fare_estimate.EstimateResult{
		TotalAmount: 23.28,
		Currency:    "EUR",
	}, nil
}

// TestInitiateFlexAuctionAuthorizedRejectsChangedRoute verifies that
// an estimate cannot be persisted after its ride coordinates change.
func TestInitiateFlexAuctionAuthorizedRejectsChangedRoute(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexAuctionTestRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeFlex,
		time.Now().UTC().Add(time.Hour),
	)

	estimator := &flexAuctionBlockingEstimator{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	service := NewService(Dependencies{
		DB:            db,
		UserRoles:     repository.NewUserRoleRepository(db),
		FareEstimator: estimator,
	})

	type outcome struct {
		result *InitiateFlexAuctionResult
		err    error
	}

	results := make(chan outcome, 1)

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		result, err := service.InitiateFlexAuctionAuthorized(
			callCtx,
			rideID,
			customerID,
			2328,
		)
		results <- outcome{result: result, err: err}
	}()

	select {
	case <-estimator.started:
	case <-callCtx.Done():
		t.Fatalf("fare estimation did not start: %v", callCtx.Err())
	}

	// The estimate is blocked. Change the authoritative route before
	// the service enters its database transaction.
	_, err := db.Exec(ctx, `
		UPDATE ride_requests
		SET destination_latitude = 60.2000,
		    updated_at = NOW()
		WHERE id = $1
	`, rideID)
	if err != nil {
		close(estimator.release)
		t.Fatalf("change ride destination: %v", err)
	}

	close(estimator.release)

	select {
	case got := <-results:
		if !errors.Is(got.err, ErrFlexAuctionNotEligible) {
			t.Fatalf(
				"expected stale-route rejection, got %v",
				got.err,
			)
		}
		if got.result != nil {
			t.Fatal("stale estimate returned an auction")
		}

	case <-callCtx.Done():
		t.Fatalf("stale-estimate initiation timed out: %v", callCtx.Err())
	}

	assertFlexAuctionCount(t, db, rideID, 0)
}
