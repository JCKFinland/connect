package dispatch

import (
	"context"
	"errors"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestBeginStandardDispatchPersistsAndPreservesMarker(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer fixture: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer fixture: %v", err)
		}
	}()

	_, err = db.Exec(ctx, `
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
			passenger_count,
			status,
			notes,
			requested_at,
			expires_at,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2,
			'Dispatch Marker Test Pickup',
			60.2055, 24.6559,
			'Dispatch Marker Test Destination',
			60.1719, 24.9414,
			'STANDARD',
			1,
			'PENDING',
			'Standard dispatch marker integration test',
			$3,
			$4,
			$3,
			$3
		)
	`, rideID, customerID, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create test ride: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if err != nil {
			t.Errorf("cleanup test ride: %v", err)
		}
	}()

	service := NewService(Dependencies{DB: db})

	var initialMarker *time.Time

	if err := db.QueryRow(ctx, `
		SELECT standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(&initialMarker); err != nil {
		t.Fatal(err)
	}

	if initialMarker != nil {
		t.Fatal("new ride unexpectedly has dispatch history")
	}

	if err := service.BeginStandardDispatch(ctx, rideID); err != nil {
		t.Fatalf("first dispatch start: %v", err)
	}

	var firstMarker time.Time

	if err := db.QueryRow(ctx, `
		SELECT standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(&firstMarker); err != nil {
		t.Fatalf("read first marker: %v", err)
	}

	if firstMarker.IsZero() {
		t.Fatal("first dispatch marker is zero")
	}

	if err := service.BeginStandardDispatch(ctx, rideID); err != nil {
		t.Fatalf("repeat dispatch start: %v", err)
	}

	var secondMarker time.Time

	if err := db.QueryRow(ctx, `
		SELECT standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(&secondMarker); err != nil {
		t.Fatalf("read second marker: %v", err)
	}

	if !firstMarker.Equal(secondMarker) {
		t.Fatalf(
			"dispatch marker changed: first=%v second=%v",
			firstMarker,
			secondMarker,
		)
	}

	t.Logf(
		"PASS: standard dispatch marker persisted and preserved: %v",
		firstMarker,
	)
}

func TestBeginStandardDispatchRejectsIneligibleRequests(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer fixture: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer fixture: %v", err)
		}
	}()

	tests := []struct {
		name         string
		matchingMode string
		status       string
		expiresAt    time.Time
	}{
		{
			name:         "FLEX matching mode",
			matchingMode: "FLEX",
			status:       "PENDING",
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "non-PENDING status",
			matchingMode: "STANDARD",
			status:       "CANCELLED",
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "expired request",
			matchingMode: "STANDARD",
			status:       "PENDING",
			expiresAt:    time.Now().UTC().Add(-time.Minute),
		},
	}

	service := NewService(Dependencies{DB: db})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
					'Lifecycle Test Pickup',
					60.2055, 24.6559,
					'Lifecycle Test Destination',
					60.1719, 24.9414,
					'STANDARD',
					1,
					$3,
					$4,
					'Dispatch marker rejection test',
					$5,
					$6,
					$5,
					$5
				)
			`,
				rideID,
				customerID,
				tc.status,
				tc.matchingMode,
				now,
				tc.expiresAt,
			)
			if err != nil {
				t.Fatalf("create test ride: %v", err)
			}

			defer func() {
				_, err := db.Exec(
					context.Background(),
					`DELETE FROM ride_requests WHERE id = $1`,
					rideID,
				)
				if err != nil {
					t.Errorf("cleanup test ride: %v", err)
				}
			}()

			rejectionErr := service.BeginStandardDispatch(ctx, rideID)
			if rejectionErr == nil {
				t.Fatal("expected dispatch start to be rejected")
			}

			var marker *time.Time

			err = db.QueryRow(ctx, `
				SELECT standard_dispatch_started_at
				FROM ride_requests
				WHERE id = $1
			`, rideID).Scan(&marker)
			if err != nil {
				t.Fatalf("read dispatch marker: %v", err)
			}

			if marker != nil {
				t.Fatalf(
					"rejected request unexpectedly has dispatch history: %v",
					*marker,
				)
			}

			t.Logf("PASS: rejected without creating dispatch history: %v", rejectionErr)
		})
	}
}

func TestDispatchRidePreservesMarkerWhenNoDriversAvailable(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer fixture: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer fixture: %v", err)
		}
	}()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(ctx, `
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
			'No Driver Marker Test Pickup',
			60.2055, 24.6559,
			'No Driver Marker Test Destination',
			60.1719, 24.9414,
			'STANDARD',
			1,
			'PENDING',
			'STANDARD',
			'No-driver marker durability test',
			$3,
			$4,
			$3,
			$3
		)
	`, rideID, customerID, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create test ride: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if err != nil {
			t.Errorf("cleanup test ride: %v", err)
		}
	}()

	// An extremely short heartbeat timeout makes all previously
	// recorded heartbeats stale without modifying driver records.
	cfg.Presence.HeartbeatTimeout = time.Nanosecond

	service := NewService(Dependencies{
		DB:       db,
		Config:   cfg,
		Presence: postgresrepo.NewDriverPresenceRepository(db),
	})

	_, dispatchErr := service.DispatchRide(ctx, rideID)

	if !errors.Is(dispatchErr, ErrNoAvailableDrivers) {
		t.Fatalf(
			"expected ErrNoAvailableDrivers, got: %v",
			dispatchErr,
		)
	}

	var marker *time.Time
	var status string
	var matchingMode string

	err = db.QueryRow(ctx, `
		SELECT
			standard_dispatch_started_at,
			status,
			matching_mode
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(
		&marker,
		&status,
		&matchingMode,
	)
	if err != nil {
		t.Fatalf("read ride after unsuccessful dispatch: %v", err)
	}

	if marker == nil {
		t.Fatal("dispatch marker was lost after no-driver rollback")
	}

	if status != "PENDING" {
		t.Fatalf("expected PENDING status, got %s", status)
	}

	if matchingMode != "STANDARD" {
		t.Fatalf("expected STANDARD matching mode, got %s", matchingMode)
	}

	t.Logf(
		"PASS: unsuccessful dispatch preserved marker %v",
		*marker,
	)
}

func TestCreateOfferPreservesMarkerWhenNoDriversAvailable(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer fixture: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer fixture: %v", err)
		}
	}()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(ctx, `
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
			'CreateOffer Marker Test Pickup',
			60.2055, 24.6559,
			'CreateOffer Marker Test Destination',
			60.1719, 24.9414,
			'STANDARD',
			1,
			'PENDING',
			'STANDARD',
			'CreateOffer durability test',
			$3,
			$4,
			$3,
			$3
		)
	`, rideID, customerID, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create test ride: %v", err)
	}

	defer func() {
		_, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup test ride: %v", cleanupErr)
		}
	}()

	cfg.Presence.HeartbeatTimeout = time.Nanosecond

	service := NewService(Dependencies{
		DB:       db,
		Config:   cfg,
		Presence: postgresrepo.NewDriverPresenceRepository(db),
	})

	offer, dispatchErr := service.CreateOffer(ctx, rideID, "")

	if !errors.Is(dispatchErr, ErrNoAvailableDrivers) {
		t.Fatalf(
			"expected ErrNoAvailableDrivers, got offer=%v err=%v",
			offer,
			dispatchErr,
		)
	}

	if offer != nil {
		t.Fatalf("expected no dispatch offer, got %+v", offer)
	}

	var marker *time.Time
	var status string
	var matchingMode string

	err = db.QueryRow(ctx, `
		SELECT
			standard_dispatch_started_at,
			status,
			matching_mode
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(
		&marker,
		&status,
		&matchingMode,
	)
	if err != nil {
		t.Fatalf("read ride after unsuccessful offer: %v", err)
	}

	if marker == nil {
		t.Fatal("CreateOffer lost dispatch marker after no-driver rollback")
	}

	if status != "PENDING" {
		t.Fatalf("expected PENDING status, got %s", status)
	}

	if matchingMode != "STANDARD" {
		t.Fatalf("expected STANDARD matching mode, got %s", matchingMode)
	}

	t.Logf(
		"PASS: CreateOffer preserved marker %v after no-driver rollback",
		*marker,
	)
}

func TestCreateOfferExpiresRideWhenExpiryChangesAfterPreflight(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer fixture: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer fixture: %v", err)
		}
	}()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(ctx, `
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
			'Expiry Race Pickup',
			60.2055, 24.6559,
			'Expiry Race Destination',
			60.1719, 24.9414,
			'STANDARD',
			1,
			'PENDING',
			'STANDARD',
			'Deterministic expiry race regression',
			$3,
			$4,
			$3,
			$3
		)
	`, rideID, customerID, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create test ride: %v", err)
	}

	defer func() {
		_, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup test ride: %v", cleanupErr)
		}
	}()

	cfg.Presence.HeartbeatTimeout = time.Nanosecond

	service := NewService(Dependencies{
		DB:       db,
		Config:   cfg,
		Presence: postgresrepo.NewDriverPresenceRepository(db),
	})

	preflightRead := make(chan struct{})
	continueDispatch := make(chan struct{})

	service.afterOfferExpiryPreflight = func(id string) {
		close(preflightRead)
		<-continueDispatch
	}

	type result struct {
		offer *models.DispatchOffer
		err   error
	}

	done := make(chan result, 1)

	go func() {
		offer, err := service.CreateOffer(ctx, rideID, "")
		done <- result{offer: offer, err: err}
	}()

	select {
	case <-preflightRead:
	case <-time.After(15 * time.Second):
		close(continueDispatch)
		t.Fatal("CreateOffer did not reach expiry preflight hook")
	}

	_, err = db.Exec(ctx, `
		UPDATE ride_requests
		SET expires_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, rideID)

	close(continueDispatch)

	if err != nil {
		t.Fatalf("expire ride during preflight: %v", err)
	}

	var dispatchResult result

	select {
	case dispatchResult = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("CreateOffer did not complete after expiry update")
	}

	if !errors.Is(dispatchResult.err, ErrRideRequestExpired) {
		t.Fatalf(
			"expected ErrRideRequestExpired, got offer=%v err=%v",
			dispatchResult.offer,
			dispatchResult.err,
		)
	}

	if dispatchResult.offer != nil {
		t.Fatalf("unexpected offer: %+v", dispatchResult.offer)
	}

	var status string
	var marker *time.Time
	var offerCount int

	err = db.QueryRow(ctx, `
		SELECT status, standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(&status, &marker)
	if err != nil {
		t.Fatalf("read expired ride: %v", err)
	}

	if status != "EXPIRED" {
		t.Fatalf("expected EXPIRED status, got %s", status)
	}

	if marker != nil {
		t.Fatalf("expired ride unexpectedly marked for dispatch: %v", marker)
	}

	err = db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM dispatch_offers
		WHERE ride_request_id = $1
	`, rideID).Scan(&offerCount)
	if err != nil {
		t.Fatalf("count dispatch offers: %v", err)
	}

	if offerCount != 0 {
		t.Fatalf("expected zero offers, got %d", offerCount)
	}

	t.Log("PASS: expiry race committed EXPIRED without dispatch history or offers")
}
