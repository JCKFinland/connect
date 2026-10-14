package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

	var customerID string
	err = db.QueryRow(ctx, `
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
		t.Fatal(err)
	}

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

	var pricingProfileID string
	err = db.QueryRow(ctx, `
		SELECT id
		FROM fare_pricing_profiles
		WHERE is_active = TRUE
		ORDER BY id
		LIMIT 1
	`).Scan(&pricingProfileID)
	if err != nil {
		t.Fatalf("resolve FLEX test pricing profile: %v", err)
	}

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
			$5, 'flex-test-v1',
			'OPEN', $3, $4, $3, $3
		)
	`, auctionID, rideID,
		now, now.Add(60*time.Second),
		pricingProfileID,
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
