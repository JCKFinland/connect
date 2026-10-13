package ride_request

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/dispatch"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupFlexActivationTest(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../../.."); err != nil {
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

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	var customerID string
	err = db.QueryRow(
		context.Background(),
		`
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
		`,
	).Scan(&customerID)
	if err != nil {
		t.Fatalf("resolve test customer: %v", err)
	}

	return db, customerID
}

func insertFlexActivationRide(
	t *testing.T,
	db *pgxpool.Pool,
	customerID string,
	status string,
	matchingMode string,
	expiresAt time.Time,
) string {
	t.Helper()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err := db.Exec(
		context.Background(),
		`
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
			'FLEX Activation Test Pickup',
			60.2055, 24.6559,
			'FLEX Activation Test Destination',
			60.1719, 24.9414,
			'STANDARD',
			1,
			$3,
			$4,
			'FLEX activation integration test',
			$5,
			$6,
			$5,
			$5
		)
		`,
		rideID,
		customerID,
		status,
		matchingMode,
		now,
		expiresAt,
	)
	if err != nil {
		t.Fatalf("insert test ride: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if err != nil {
			t.Errorf("cleanup test ride: %v", err)
		}
	})

	return rideID
}

func TestActivateFlexAuthorizedSucceeds(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexActivationRide(
		t, db, customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	if err := service.ActivateFlexAuthorized(ctx, rideID, customerID); err != nil {
		t.Fatalf("activate FLEX: %v", err)
	}

	var status, matchingMode string
	var marker *time.Time

	err := db.QueryRow(ctx, `
		SELECT status, matching_mode, standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
	`, rideID).Scan(&status, &matchingMode, &marker)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" {
		t.Fatalf("status changed unexpectedly: %s", status)
	}
	if matchingMode != models.RideMatchingModeFlex {
		t.Fatalf("expected FLEX, got %s", matchingMode)
	}
	if marker != nil {
		t.Fatal("activation unexpectedly created STANDARD dispatch history")
	}
}

func TestActivateFlexAuthorizedRejectsNonOwner(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexActivationRide(
		t, db, customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	err := service.ActivateFlexAuthorized(
		ctx,
		rideID,
		uuid.NewString(),
	)

	if !errors.Is(err, ErrFlexActivationAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}

	var mode string
	if err := db.QueryRow(ctx,
		`SELECT matching_mode FROM ride_requests WHERE id = $1`,
		rideID,
	).Scan(&mode); err != nil {
		t.Fatal(err)
	}

	if mode != models.RideMatchingModeStandard {
		t.Fatalf("unauthorized activation changed mode to %s", mode)
	}
}

func TestActivateFlexAuthorizedRejectsStandardDispatchHistory(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexActivationRide(
		t, db, customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	_, err := db.Exec(ctx, `
		UPDATE ride_requests
		SET standard_dispatch_started_at = NOW()
		WHERE id = $1
	`, rideID)
	if err != nil {
		t.Fatalf("set STANDARD dispatch history: %v", err)
	}

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	err = service.ActivateFlexAuthorized(ctx, rideID, customerID)
	if !errors.Is(err, ErrFlexActivationNotEligible) {
		t.Fatalf("expected not eligible, got %v", err)
	}

	var mode string
	var marker *time.Time

	if err := db.QueryRow(ctx, `
		SELECT matching_mode, standard_dispatch_started_at
		FROM ride_requests WHERE id = $1
	`, rideID).Scan(&mode, &marker); err != nil {
		t.Fatal(err)
	}

	if mode != models.RideMatchingModeStandard || marker == nil {
		t.Fatalf("dispatch history protection failed: mode=%s marker=%v", mode, marker)
	}
}

// TestActivateFlexAuthorizedRejectsIneligibleLifecycleStates verifies
// that only unexpired, PENDING, STANDARD requests can activate FLEX.
func TestActivateFlexAuthorizedRejectsIneligibleLifecycleStates(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	tests := []struct {
		name         string
		status       string
		matchingMode string
		expiresAt    time.Time
	}{
		{
			name:         "expired STANDARD ride",
			status:       "PENDING",
			matchingMode: models.RideMatchingModeStandard,
			expiresAt:    time.Now().UTC().Add(-time.Minute),
		},
		{
			name:         "cancelled STANDARD ride",
			status:       "CANCELLED",
			matchingMode: models.RideMatchingModeStandard,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "matching STANDARD ride",
			status:       "MATCHING",
			matchingMode: models.RideMatchingModeStandard,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "accepted STANDARD ride",
			status:       "ACCEPTED",
			matchingMode: models.RideMatchingModeStandard,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
		{
			name:         "already FLEX ride",
			status:       "PENDING",
			matchingMode: models.RideMatchingModeFlex,
			expiresAt:    time.Now().UTC().Add(time.Hour),
		},
	}

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rideID := insertFlexActivationRide(
				t,
				db,
				customerID,
				tc.status,
				tc.matchingMode,
				tc.expiresAt,
			)

			err := service.ActivateFlexAuthorized(
				ctx,
				rideID,
				customerID,
			)

			if !errors.Is(err, ErrFlexActivationNotEligible) {
				t.Fatalf(
					"expected not eligible, got %v",
					err,
				)
			}

			var status, matchingMode string

			if err := db.QueryRow(
				ctx,
				`
				SELECT status, matching_mode
				FROM ride_requests
				WHERE id = $1
				`,
				rideID,
			).Scan(&status, &matchingMode); err != nil {
				t.Fatal(err)
			}

			if status != tc.status {
				t.Fatalf(
					"status changed: expected %s, got %s",
					tc.status,
					status,
				)
			}

			if matchingMode != tc.matchingMode {
				t.Fatalf(
					"matching mode changed: expected %s, got %s",
					tc.matchingMode,
					matchingMode,
				)
			}
		})
	}
}

// TestActivateFlexAuthorizedRejectsOwnerWithoutCustomerRole verifies
// that ownership alone does not authorize FLEX activation.
func TestActivateFlexAuthorizedRejectsOwnerWithoutCustomerRole(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexActivationRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB: db,
		UserRoles: &createUserRoleTestRepository{
			roles: []string{"DRIVER"},
		},
	})

	err := service.ActivateFlexAuthorized(
		ctx,
		rideID,
		customerID,
	)

	if !errors.Is(err, ErrFlexActivationAccessDenied) {
		t.Fatalf(
			"expected CUSTOMER role access denied, got %v",
			err,
		)
	}

	var matchingMode string

	err = db.QueryRow(
		ctx,
		`
		SELECT matching_mode
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(&matchingMode)
	if err != nil {
		t.Fatal(err)
	}

	if matchingMode != models.RideMatchingModeStandard {
		t.Fatalf(
			"unauthorized owner changed matching mode to %s",
			matchingMode,
		)
	}
}

// TestActivateFlexAuthorizedConcurrentAttempts verifies that concurrent
// requests cannot activate the same STANDARD ride more than once.
func TestActivateFlexAuthorizedConcurrentAttempts(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx := context.Background()

	rideID := insertFlexActivationRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	const attempts = 2

	start := make(chan struct{})
	results := make(chan error, attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			<-start

			results <- service.ActivateFlexAuthorized(
				ctx,
				rideID,
				customerID,
			)
		}()
	}

	close(start)

	successes := 0
	ineligible := 0

	for i := 0; i < attempts; i++ {
		select {
		case err := <-results:
			switch {
			case err == nil:
				successes++

			case errors.Is(err, ErrFlexActivationNotEligible):
				ineligible++

			default:
				t.Fatalf(
					"unexpected concurrent activation error: %v",
					err,
				)
			}

		case <-time.After(10 * time.Second):
			t.Fatal("concurrent activation timed out")
		}
	}

	if successes != 1 {
		t.Fatalf(
			"expected exactly one successful activation, got %d",
			successes,
		)
	}

	if ineligible != 1 {
		t.Fatalf(
			"expected exactly one ineligible result, got %d",
			ineligible,
		)
	}

	var status, matchingMode string
	var marker *time.Time

	err := db.QueryRow(
		ctx,
		`
		SELECT status, matching_mode, standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(&status, &matchingMode, &marker)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" {
		t.Fatalf("unexpected final status: %s", status)
	}

	if matchingMode != models.RideMatchingModeFlex {
		t.Fatalf(
			"expected final FLEX mode, got %s",
			matchingMode,
		)
	}

	if marker != nil {
		t.Fatal("concurrent activation created STANDARD dispatch history")
	}
}

// TestActivateFlexAuthorizedLosesToStandardDispatch verifies that
// FLEX activation cannot bypass a STANDARD dispatch marker committed
// while activation is waiting for the dispatch advisory lock.
func TestActivateFlexAuthorizedLosesToStandardDispatch(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	rideID := insertFlexActivationRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	// Simulate the critical transaction section of STANDARD dispatch.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	_, err = tx.Exec(
		ctx,
		`
		SELECT pg_advisory_xact_lock(
			hashtextextended($1, 0)
		)
		`,
		"ride-dispatch:"+rideID,
	)
	if err != nil {
		t.Fatal(err)
	}

	activationDone := make(chan error, 1)

	go func() {
		activationDone <- service.ActivateFlexAuthorized(
			ctx,
			rideID,
			customerID,
		)
	}()

	// Commit durable dispatch history before releasing the advisory lock.
	result, err := tx.Exec(
		ctx,
		`
		UPDATE ride_requests
		SET standard_dispatch_started_at = NOW()
		WHERE id = $1
		  AND matching_mode = 'STANDARD'
		  AND status = 'PENDING'
		  AND standard_dispatch_started_at IS NULL
		`,
		rideID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.RowsAffected() != 1 {
		t.Fatalf(
			"expected one STANDARD dispatch marker update, got %d",
			result.RowsAffected(),
		)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-activationDone:
		if !errors.Is(err, ErrFlexActivationNotEligible) {
			t.Fatalf(
				"expected FLEX activation rejection after STANDARD dispatch, got %v",
				err,
			)
		}

	case <-ctx.Done():
		t.Fatalf("activation did not finish: %v", ctx.Err())
	}

	var matchingMode string
	var marker *time.Time

	err = db.QueryRow(
		ctx,
		`
		SELECT matching_mode, standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(&matchingMode, &marker)
	if err != nil {
		t.Fatal(err)
	}

	if matchingMode != models.RideMatchingModeStandard {
		t.Fatalf(
			"expected STANDARD matching mode, got %s",
			matchingMode,
		)
	}

	if marker == nil {
		t.Fatal("STANDARD dispatch marker was not preserved")
	}
}

// TestActivateFlexAuthorizedPreventsStandardDispatch verifies that
// committed FLEX activation prevents subsequent STANDARD dispatch.
func TestActivateFlexAuthorizedPreventsStandardDispatch(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	rideID := insertFlexActivationRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	flexService := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	dispatchService := dispatch.NewService(
		dispatch.Dependencies{
			DB: db,
		},
	)

	// FLEX wins by committing its matching-mode transition first.
	if err := flexService.ActivateFlexAuthorized(
		ctx,
		rideID,
		customerID,
	); err != nil {
		t.Fatalf("activate FLEX: %v", err)
	}

	// The actual STANDARD dispatch service must reject this ride.
	err := dispatchService.BeginStandardDispatch(ctx, rideID)
	if err == nil {
		t.Fatal("STANDARD dispatch unexpectedly accepted a FLEX ride")
	}

	var status, matchingMode string
	var marker *time.Time

	err = db.QueryRow(
		ctx,
		`
		SELECT
			status,
			matching_mode,
			standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(&status, &matchingMode, &marker)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" {
		t.Fatalf("unexpected ride status: %s", status)
	}

	if matchingMode != models.RideMatchingModeFlex {
		t.Fatalf(
			"expected FLEX matching mode, got %s",
			matchingMode,
		)
	}

	if marker != nil {
		t.Fatal("STANDARD dispatch history was created after FLEX activation")
	}
}

// TestActivateFlexAuthorizedWaitsForDispatchLock verifies that an
// in-flight FLEX activation waits for an existing dispatch advisory
// lock and then observes the committed STANDARD dispatch marker.
func TestActivateFlexAuthorizedWaitsForDispatchLock(t *testing.T) {
	db, customerID := setupFlexActivationTest(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	rideID := insertFlexActivationRide(
		t,
		db,
		customerID,
		"PENDING",
		models.RideMatchingModeStandard,
		time.Now().UTC().Add(time.Hour),
	)

	service := NewService(Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	_, err = tx.Exec(
		ctx,
		`
		SELECT pg_advisory_xact_lock(
			hashtextextended($1, 0)
		)
		`,
		"ride-dispatch:"+rideID,
	)
	if err != nil {
		t.Fatal(err)
	}

	activationDone := make(chan error, 1)

	go func() {
		activationDone <- service.ActivateFlexAuthorized(
			ctx,
			rideID,
			customerID,
		)
	}()

	// Confirm that the activation transaction is actually waiting
	// for an advisory lock held by another PostgreSQL transaction.
	waiting := false
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		var count int

		err = db.QueryRow(
			ctx,
			`
			SELECT COUNT(*)
			FROM pg_locks
			WHERE locktype = 'advisory'
			  AND granted = false
			`,
		).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}

		if count > 0 {
			waiting = true
			break
		}

		select {
		case activationErr := <-activationDone:
			t.Fatalf(
				"activation completed before dispatch lock release: %v",
				activationErr,
			)
		default:
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !waiting {
		t.Fatal("FLEX activation was not observed waiting for advisory lock")
	}

	// Persist dispatch history while the competing activation waits.
	result, err := tx.Exec(
		ctx,
		`
		UPDATE ride_requests
		SET standard_dispatch_started_at = NOW()
		WHERE id = $1
		  AND status = 'PENDING'
		  AND matching_mode = 'STANDARD'
		  AND standard_dispatch_started_at IS NULL
		`,
		rideID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.RowsAffected() != 1 {
		t.Fatalf(
			"expected one dispatch marker update, got %d",
			result.RowsAffected(),
		)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case activationErr := <-activationDone:
		if !errors.Is(
			activationErr,
			ErrFlexActivationNotEligible,
		) {
			t.Fatalf(
				"expected FLEX rejection after lock release, got %v",
				activationErr,
			)
		}

	case <-ctx.Done():
		t.Fatalf("activation timed out: %v", ctx.Err())
	}

	var matchingMode string
	var marker *time.Time

	err = db.QueryRow(
		ctx,
		`
		SELECT matching_mode, standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(&matchingMode, &marker)
	if err != nil {
		t.Fatal(err)
	}

	if matchingMode != models.RideMatchingModeStandard {
		t.Fatalf(
			"expected STANDARD matching mode, got %s",
			matchingMode,
		)
	}

	if marker == nil {
		t.Fatal("dispatch marker was not preserved")
	}
}
