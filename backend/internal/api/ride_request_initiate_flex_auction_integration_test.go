package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	ride_request "github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type flexAuctionHTTPFareEstimator struct {
	result *fare_estimate.EstimateResult
}

func (s *flexAuctionHTTPFareEstimator) Estimate(
	_ context.Context,
	_ fare_estimate.EstimateRequest,
) (*fare_estimate.EstimateResult, error) {
	return s.result, nil
}

func setupFlexAuctionHTTPDatabase(
	t *testing.T,
) (*pgxpool.Pool, string) {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}

	cfg, loadErr := config.Load()

	if err := os.Chdir(originalDir); err != nil {
		t.Fatal(err)
	}

	if loadErr != nil {
		t.Fatal(loadErr)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	var customerID string
	err = db.QueryRow(context.Background(), `
		SELECT u.id
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles r ON r.id = ur.role_id
		WHERE u.deleted_at IS NULL
		GROUP BY u.id
		HAVING BOOL_OR(r.name = 'CUSTOMER')
		   AND COUNT(*) FILTER (WHERE r.name <> 'CUSTOMER') = 0
		ORDER BY u.id
		LIMIT 1
	`).Scan(&customerID)
	if err != nil {
		t.Fatalf("resolve test customer: %v", err)
	}

	return db, customerID
}

func insertFlexAuctionHTTPRide(
	t *testing.T,
	db *pgxpool.Pool,
	customerID string,
	categoryID string,
) string {
	t.Helper()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err := db.Exec(context.Background(), `
		INSERT INTO ride_requests (
			id, customer_id,
			pickup_address, pickup_latitude, pickup_longitude,
			destination_address, destination_latitude, destination_longitude,
			requested_vehicle_type, service_category_id,
			passenger_count, status, matching_mode,
			notes, requested_at, expires_at,
			created_at, updated_at
		)
		VALUES (
			$1, $2,
			'FLEX HTTP Test Pickup', 60.2055, 24.6559,
			'FLEX HTTP Test Destination', 60.1719, 24.9414,
			'STANDARD', $3,
			1, 'PENDING', 'FLEX',
			'Disposable FLEX HTTP integration test',
			$4, $5, $4, $4
		)
	`,
		rideID,
		customerID,
		categoryID,
		now,
		now.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("insert FLEX HTTP ride: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()

		if _, err := db.Exec(ctx,
			`DELETE FROM flex_auctions WHERE ride_request_id = $1`,
			rideID,
		); err != nil {
			t.Errorf("cleanup FLEX auctions: %v", err)
		}

		if _, err := db.Exec(ctx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		); err != nil {
			t.Errorf("cleanup FLEX ride: %v", err)
		}
	})

	return rideID
}

func newFlexAuctionHTTPIntegrationRouter(
	customerID string,
	service *ride_request.Service,
) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	router.Use(func(c *gin.Context) {
		user := &models.User{}
		user.ID = customerID
		middleware.SetCurrentUser(c, user)
		c.Next()
	})

	handler := NewRideRequestHandler(service)

	router.POST(
		flexAuctionTestRoute,
		handler.InitiateFlexAuction,
	)

	return router
}

func TestInitiateFlexAuctionHTTPIntegrationSucceeds(
	t *testing.T,
) {
	db, customerID := setupFlexAuctionHTTPDatabase(t)
	ctx := context.Background()

	var categoryID string
	if err := db.QueryRow(ctx, `
		SELECT id
		FROM service_categories
		WHERE is_active = TRUE
		ORDER BY id
		LIMIT 1
	`).Scan(&categoryID); err != nil {
		t.Fatalf("resolve service category: %v", err)
	}

	var companyID, branchID string
	if err := db.QueryRow(ctx, `
		SELECT c.id, b.id
		FROM companies c
		JOIN branches b ON b.company_id = c.id
		WHERE c.deleted_at IS NULL
		  AND b.deleted_at IS NULL
		ORDER BY c.id, b.id
		LIMIT 1
	`).Scan(&companyID, &branchID); err != nil {
		t.Fatalf("resolve company and branch: %v", err)
	}

	profileID := uuid.NewString()
	version := "flex-http-" + uuid.NewString()[:8]
	now := time.Now().UTC()

	_, err := db.Exec(ctx, `
		INSERT INTO fare_pricing_profiles (
			id, company_id, branch_id,
			service_category_id, version, currency,
			base_fare, distance_rate_per_km,
			time_rate_per_minute, waiting_rate_per_minute,
			booking_fee, surge_multiplier,
			effective_from, effective_to,
			is_active, created_at
		)
		VALUES (
			$1, $2, $3, $4, $5, 'EUR',
			4.90, 1.50, 0.25, 0.50,
			2.00, 1.00,
			$6, NULL, TRUE, $6
		)
	`,
		profileID,
		companyID,
		branchID,
		categoryID,
		version,
		now,
	)
	if err != nil {
		t.Fatalf("insert pricing profile: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM fare_pricing_profiles WHERE id = $1`,
			profileID,
		); err != nil {
			t.Errorf("cleanup pricing profile: %v", err)
		}
	})

	rideID := insertFlexAuctionHTTPRide(
		t, db, customerID, categoryID,
	)

	estimator := &flexAuctionHTTPFareEstimator{
		result: &fare_estimate.EstimateResult{
			TotalAmount:      23.28,
			Currency:         "EUR",
			PricingProfileID: profileID,
			PricingVersion:   version,
		},
	}

	service := ride_request.NewService(
		ride_request.Dependencies{
			DB:            db,
			UserRoles:     repository.NewUserRoleRepository(db),
			FareEstimator: estimator,
		},
	)

	router := newFlexAuctionHTTPIntegrationRouter(
		customerID,
		service,
	)

	before := time.Now().UTC()

	recorder := executeFlexAuctionRequest(
		router,
		rideID,
		`{"passenger_offer_cents":2328}`,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected HTTP 201, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Auction           models.FlexAuction `json:"auction"`
			BaselineCents     int64              `json:"baseline_cents"`
			MinimumOfferCents int64              `json:"minimum_offer_cents"`
			MaximumOfferCents int64              `json:"maximum_offer_cents"`
			PricingProfileID  string             `json:"pricing_profile_id"`
			PricingVersion    string             `json:"pricing_version"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf("decode FLEX auction response: %v", err)
	}

	if !body.Success {
		t.Fatal("HTTP 201 response marked unsuccessful")
	}

	auction := body.Data.Auction

	if auction.ID == "" ||
		auction.RideRequestID != rideID ||
		auction.PassengerOfferCents != 2328 ||
		auction.Currency != "EUR" ||
		auction.Status != models.FlexAuctionStatusOpen {
		t.Fatalf("unexpected auction response: %+v", auction)
	}

	if body.Data.BaselineCents != 2328 ||
		body.Data.MinimumOfferCents != 2095 ||
		body.Data.MaximumOfferCents != 2561 {
		t.Fatalf("unexpected FLEX pricing bounds: %+v", body.Data)
	}

	if body.Data.PricingProfileID != profileID ||
		body.Data.PricingVersion != version {
		t.Fatalf("incorrect pricing provenance: %+v", body.Data)
	}

	if auction.ExpiresAt.Before(before.Add(59*time.Second)) ||
		auction.ExpiresAt.After(time.Now().UTC().Add(61*time.Second)) {
		t.Fatalf("unexpected auction expiry: %v", auction.ExpiresAt)
	}

	var count int
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM flex_auctions
		WHERE ride_request_id = $1
		  AND passenger_offer_amount = 23.28
		  AND pricing_profile_id = $2
		  AND pricing_version = $3
	`,
		rideID,
		profileID,
		version,
	).Scan(&count); err != nil {
		t.Fatalf("verify persisted auction: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one persisted auction, got %d", count)
	}
}

// TestInitiateFlexAuctionHTTPIntegrationRejections verifies that
// rejected HTTP requests cannot create unauthorized or invalid auctions.
func TestInitiateFlexAuctionHTTPIntegrationRejections(t *testing.T) {
	db, customerID := setupFlexAuctionHTTPDatabase(t)
	ctx := context.Background()

	var categoryID string
	if err := db.QueryRow(ctx, `
		SELECT id FROM service_categories
		WHERE is_active = TRUE
		ORDER BY id LIMIT 1
	`).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}

	var profileID, version string
	if err := db.QueryRow(ctx, `
		SELECT id, version
		FROM fare_pricing_profiles
		WHERE is_active = TRUE
		  AND currency = 'EUR'
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&profileID, &version); err != nil {
		t.Fatal(err)
	}

	estimator := &flexAuctionHTTPFareEstimator{
		result: &fare_estimate.EstimateResult{
			TotalAmount:      23.28,
			Currency:         "EUR",
			PricingProfileID: profileID,
			PricingVersion:   version,
		},
	}

	service := ride_request.NewService(
		ride_request.Dependencies{
			DB:            db,
			UserRoles:     repository.NewUserRoleRepository(db),
			FareEstimator: estimator,
		},
	)

	assertCount := func(rideID string, expected int) {
		t.Helper()

		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM flex_auctions
			WHERE ride_request_id = $1
		`, rideID).Scan(&count); err != nil {
			t.Fatal(err)
		}

		if count != expected {
			t.Fatalf(
				"ride %s: expected %d auctions, got %d",
				rideID, expected, count,
			)
		}
	}

	t.Run("unauthorized ownership", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		router := newFlexAuctionHTTPIntegrationRouter(
			uuid.NewString(),
			service,
		)

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusForbidden,
		)
		assertCount(rideID, 0)
	})

	t.Run("offer below minimum", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		router := newFlexAuctionHTTPIntegrationRouter(
			customerID, service,
		)

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2094}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusUnprocessableEntity,
		)
		assertCount(rideID, 0)
	})

	t.Run("offer above maximum", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		router := newFlexAuctionHTTPIntegrationRouter(
			customerID, service,
		)

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2562}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusUnprocessableEntity,
		)
		assertCount(rideID, 0)
	})

	t.Run("duplicate auction", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		router := newFlexAuctionHTTPIntegrationRouter(
			customerID, service,
		)

		first := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		if first.Code != http.StatusCreated {
			t.Fatalf(
				"initial auction: expected 201, got %d: %s",
				first.Code, first.Body.String(),
			)
		}

		assertCount(rideID, 1)

		second := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		assertFlexAuctionHTTPFailure(
			t, second, http.StatusConflict,
		)
		assertCount(rideID, 1)
	})

	t.Run("nonexistent ride request", func(t *testing.T) {
		rideID := uuid.NewString()

		router := newFlexAuctionHTTPIntegrationRouter(
			customerID, service,
		)

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusNotFound,
		)
		assertCount(rideID, 0)
	})
}

// TestInitiateFlexAuctionHTTPIntegrationEligibility verifies ride
// eligibility and the authoritative auction expiry deadline.
func TestInitiateFlexAuctionHTTPIntegrationEligibility(t *testing.T) {
	db, customerID := setupFlexAuctionHTTPDatabase(t)
	ctx := context.Background()

	var categoryID string
	if err := db.QueryRow(ctx, `
		SELECT id
		FROM service_categories
		WHERE is_active = TRUE
		ORDER BY id
		LIMIT 1
	`).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}

	var profileID, version string
	if err := db.QueryRow(ctx, `
		SELECT id, version
		FROM fare_pricing_profiles
		WHERE is_active = TRUE
		  AND currency = 'EUR'
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&profileID, &version); err != nil {
		t.Fatal(err)
	}

	estimator := &flexAuctionHTTPFareEstimator{
		result: &fare_estimate.EstimateResult{
			TotalAmount:      23.28,
			Currency:         "EUR",
			PricingProfileID: profileID,
			PricingVersion:   version,
		},
	}

	service := ride_request.NewService(
		ride_request.Dependencies{
			DB:            db,
			UserRoles:     repository.NewUserRoleRepository(db),
			FareEstimator: estimator,
		},
	)

	router := newFlexAuctionHTTPIntegrationRouter(
		customerID, service,
	)

	assertAuctionCount := func(t *testing.T, rideID string, expected int) {
		t.Helper()

		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM flex_auctions
			WHERE ride_request_id = $1
		`, rideID).Scan(&count); err != nil {
			t.Fatal(err)
		}

		if count != expected {
			t.Fatalf(
				"expected %d auctions, got %d",
				expected, count,
			)
		}
	}

	t.Run("non pending ride", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		_, err := db.Exec(ctx, `
			UPDATE ride_requests
			SET status = 'CANCELLED'
			WHERE id = $1
		`, rideID)
		if err != nil {
			t.Fatal(err)
		}

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusConflict,
		)
		assertAuctionCount(t, rideID, 0)
	})

	t.Run("non FLEX matching mode", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		_, err := db.Exec(ctx, `
			UPDATE ride_requests
			SET matching_mode = 'STANDARD'
			WHERE id = $1
		`, rideID)
		if err != nil {
			t.Fatal(err)
		}

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusConflict,
		)
		assertAuctionCount(t, rideID, 0)
	})

	t.Run("expired ride request", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		_, err := db.Exec(ctx, `
			UPDATE ride_requests
			SET expires_at = NOW() - INTERVAL '1 second'
			WHERE id = $1
		`, rideID)
		if err != nil {
			t.Fatal(err)
		}

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		assertFlexAuctionHTTPFailure(
			t, recorder, http.StatusConflict,
		)
		assertAuctionCount(t, rideID, 0)
	})

	t.Run("auction expiry capped by ride deadline", func(t *testing.T) {
		rideID := insertFlexAuctionHTTPRide(
			t, db, customerID, categoryID,
		)

		deadline := time.Now().UTC().Add(30 * time.Second).
			Truncate(time.Microsecond)

		_, err := db.Exec(ctx, `
			UPDATE ride_requests
			SET expires_at = $2
			WHERE id = $1
		`, rideID, deadline)
		if err != nil {
			t.Fatal(err)
		}

		recorder := executeFlexAuctionRequest(
			router, rideID,
			`{"passenger_offer_cents":2328}`,
		)

		if recorder.Code != http.StatusCreated {
			t.Fatalf(
				"expected HTTP 201, got %d: %s",
				recorder.Code, recorder.Body.String(),
			)
		}

		var auctionExpiry time.Time
		if err := db.QueryRow(ctx, `
			SELECT expires_at
			FROM flex_auctions
			WHERE ride_request_id = $1
		`, rideID).Scan(&auctionExpiry); err != nil {
			t.Fatal(err)
		}

		if !auctionExpiry.Equal(deadline) {
			t.Fatalf(
				"expected auction expiry %v, got %v",
				deadline, auctionExpiry,
			)
		}

		assertAuctionCount(t, rideID, 1)
	})
}
