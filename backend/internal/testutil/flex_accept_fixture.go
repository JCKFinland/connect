package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// flexFixtureConfigMu serializes the temporary working-directory change
// required by the existing config.Load implementation.
var flexFixtureConfigMu sync.Mutex

func loadFlexFixtureConfig(t *testing.T) *config.Config {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve FLEX fixture source location")
	}

	backendRoot := filepath.Clean(
		filepath.Join(filepath.Dir(filename), "../.."),
	)

	flexFixtureConfigMu.Lock()
	defer flexFixtureConfigMu.Unlock()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(backendRoot); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	return cfg
}

type FlexAcceptFixture struct {
	DB           *pgxpool.Pool
	Config       *config.Config
	DriverID     string
	AuctionID    string
	RideID       string
	AssignmentID string
}

func SetupFlexAcceptFixture(t *testing.T) *FlexAcceptFixture {
	t.Helper()

	ctx := context.Background()

	cfg := loadFlexFixtureConfig(t)

	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") == "1" {
		const requiredDatabase = "connect_settlement_test"

		if cfg.Database.Name != requiredDatabase {
			t.Fatalf(
				"settlement integration test requires database %q; configured database is %q",
				requiredDatabase,
				cfg.Database.Name,
			)
		}
	}

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatal("invalid heartbeat timeout")
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	driver, cleanupDriver, err :=
		CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	cleanupCompliance, err :=
		MakeDriverRegulatorilyCompliant(ctx, db, driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanupCompliance(context.Background()); err != nil {
			t.Errorf("cleanup compliance: %v", err)
		}
	})

	var driverRoleID string
	err = db.QueryRow(ctx,
		`SELECT id FROM roles WHERE name = 'DRIVER' LIMIT 1`,
	).Scan(&driverRoleID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id)
		 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		driver.UserID, driverRoleID,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`,
			driver.UserID, driverRoleID,
		)
		if err != nil {
			t.Errorf("cleanup driver role: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			vehicle_id, assignment_id,
			is_online, availability_status,
			latitude, longitude, last_heartbeat_at
		)
		VALUES (
			$1, $2, $3, $4, $5,
			TRUE, 'AVAILABLE',
			60.2055, 24.6559, NOW()
		)
	`,
		driver.UserID,
		driver.CompanyID,
		driver.BranchID,
		driver.VehicleID,
		driver.AssignmentID,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			driver.UserID,
		)
		if err != nil {
			t.Errorf("cleanup driver presence: %v", err)
		}
	})

	// Every FLEX fixture owns its passenger identity. Never borrow a
	// customer from the shared development database.
	customerID := uuid.NewString()
	customerEmail := "flex-fixture-" +
		uuid.NewString() + "@example.invalid"

	_, err = db.Exec(ctx, `
		INSERT INTO users (
			id, email, password_hash,
			first_name, last_name, phone,
			is_active, is_verified
		)
		VALUES (
			$1, $2, $3,
			'FLEX', 'Integration Test', $4,
			TRUE, TRUE
		)
	`,
		customerID,
		customerEmail,
		"flex-test-account-no-login",
		"+358"+strings.ReplaceAll(customerID, "-", "")[:12],
	)
	if err != nil {
		t.Fatalf("create FLEX fixture customer: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM users WHERE id = $1`,
			customerID,
		)
		if err != nil {
			t.Errorf("cleanup FLEX fixture customer: %v", err)
		}
	})

	var customerRoleID string
	err = db.QueryRow(ctx, `
		SELECT id
		FROM roles
		WHERE name = 'CUSTOMER'
		LIMIT 1
	`).Scan(&customerRoleID)
	if err != nil {
		t.Fatalf("resolve CUSTOMER role: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
	`,
		customerID,
		customerRoleID,
	)
	if err != nil {
		t.Fatalf("assign FLEX fixture customer role: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM user_roles
			 WHERE user_id = $1 AND role_id = $2`,
			customerID,
			customerRoleID,
		)
		if err != nil {
			t.Errorf("cleanup FLEX fixture customer role: %v", err)
		}
	})

	rideID := uuid.NewString()
	auctionID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(ctx, `
		INSERT INTO ride_requests (
			id, customer_id,
			pickup_address, pickup_latitude, pickup_longitude,
			destination_address,
			destination_latitude, destination_longitude,
			requested_vehicle_type, passenger_count,
			status, matching_mode, notes,
			requested_at, expires_at,
			created_at, updated_at
		)
		VALUES (
			$1, $2,
			'FLEX Offer Test Pickup', 60.2055, 24.6559,
			'FLEX Offer Test Destination', 60.1719, 24.9414,
			'STANDARD', 1,
			'PENDING', $3,
			'FLEX offer acceptance integration test',
			$4, $5, $4, $4
		)
	`, rideID, customerID,
		models.RideMatchingModeFlex,
		now, now.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if err != nil {
			t.Errorf("cleanup ride request: %v", err)
		}
	})

	// Create pricing provenance owned by this fixture's driver company.
	// Never borrow a pricing profile from unrelated database records.
	serviceCategoryID := uuid.NewString()
	pricingProfileID := uuid.NewString()
	pricingVersion := "flex-test-" + uuid.NewString()[:8]

	_, err = db.Exec(ctx, `
		INSERT INTO service_categories (
			id, code, name,
			is_active, created_at, updated_at
		)
		VALUES (
			$1, $2, 'FLEX Integration Test',
			TRUE, $3, $3
		)
	`,
		serviceCategoryID,
		"FLEX_"+uuid.NewString()[:8],
		now,
	)
	if err != nil {
		t.Fatalf("create FLEX test service category: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM service_categories WHERE id = $1`,
			serviceCategoryID,
		); err != nil {
			t.Errorf("cleanup FLEX service category: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
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
			$6, NULL,
			TRUE, $6
		)
	`,
		pricingProfileID,
		driver.CompanyID,
		driver.BranchID,
		serviceCategoryID,
		pricingVersion,
		now,
	)
	if err != nil {
		t.Fatalf("create FLEX test pricing profile: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM fare_pricing_profiles WHERE id = $1`,
			pricingProfileID,
		); err != nil {
			t.Errorf("cleanup FLEX pricing profile: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
		INSERT INTO flex_auctions (
			id, ride_request_id,
			passenger_offer_amount, currency,
			baseline_amount,
			minimum_offer_amount,
			maximum_offer_amount,
			pricing_profile_id,
			pricing_version,
			status, opened_at, expires_at,
			created_at, updated_at
		)
		VALUES (
			$1, $2, 23.28, 'EUR',
			23.28, 20.95, 25.61,
			$5, $6,
			'OPEN', $3, $4, $3, $3
		)
	`, auctionID, rideID,
		now, now.Add(60*time.Second),
		pricingProfileID,
		pricingVersion,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(),
			`DELETE FROM flex_offers WHERE auction_id = $1`,
			auctionID,
		); err != nil {
			t.Errorf("cleanup FLEX offers: %v", err)
		}

		if _, err := db.Exec(context.Background(),
			`DELETE FROM flex_auctions WHERE id = $1`,
			auctionID,
		); err != nil {
			t.Errorf("cleanup FLEX auction: %v", err)
		}
	})

	return &FlexAcceptFixture{
		DB:           db,
		Config:       cfg,
		DriverID:     driver.UserID,
		AuctionID:    auctionID,
		RideID:       rideID,
		AssignmentID: driver.AssignmentID,
	}
}
