package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestScheduleDispatchRetryCapsNextAttemptAtRideExpiry(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	rideRequestID := uuid.NewString()

	now := time.Now().UTC()

	// The 5th retry normally uses a 30-second backoff.
	// The ride itself will expire only 5 seconds after
	// the attempted dispatch.
	attemptedAt := now.Add(
		1 * time.Minute,
	)

	rideExpiresAt := attemptedAt.Add(
		5 * time.Second,
	)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO ride_requests
			(
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
				dispatch_retry_count,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Retry Expiry Cap Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Retry scheduling must not extend beyond ride expiry',
				$3,
				$4,
				4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		rideExpiresAt,
	)
	if err != nil {
		t.Fatalf(
			"create retry expiry-cap ride: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup retry expiry-cap ride: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewRideRequestRepository(db)

	retryCount, nextAttemptAt, err :=
		repo.ScheduleDispatchRetry(
			ctx,
			rideRequestID,
			attemptedAt,
		)
	if err != nil {
		t.Fatalf(
			"schedule capped dispatch retry: %v",
			err,
		)
	}

	if retryCount != 5 {
		t.Fatalf(
			"expected retry count 5, got %d",
			retryCount,
		)
	}

	const timestampTolerance = time.Millisecond

	difference := nextAttemptAt.Sub(
		rideExpiresAt,
	)

	if difference < 0 {
		difference = -difference
	}

	if difference > timestampTolerance {
		t.Fatalf(
			"expected next dispatch attempt about ride expiry %v, got %v",
			rideExpiresAt,
			nextAttemptAt,
		)
	}

	var (
		status                 string
		persistedRetryCount    int
		persistedNextAttemptAt *time.Time
		persistedLastAttemptAt *time.Time
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				dispatch_retry_count,
				next_dispatch_attempt_at,
				last_dispatch_attempt_at
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&status,
		&persistedRetryCount,
		&persistedNextAttemptAt,
		&persistedLastAttemptAt,
	)
	if err != nil {
		t.Fatalf(
			"read persisted retry expiry-cap state: %v",
			err,
		)
	}

	if status != "PENDING" {
		t.Fatalf(
			"expected ride to remain PENDING, got %s",
			status,
		)
	}

	if persistedRetryCount != 5 {
		t.Fatalf(
			"expected persisted retry count 5, got %d",
			persistedRetryCount,
		)
	}

	if persistedNextAttemptAt == nil {
		t.Fatal(
			"expected next_dispatch_attempt_at to be populated",
		)
	}

	persistedDifference :=
		persistedNextAttemptAt.Sub(
			rideExpiresAt,
		)

	if persistedDifference < 0 {
		persistedDifference =
			-persistedDifference
	}

	if persistedDifference > timestampTolerance {
		t.Fatalf(
			"expected persisted next attempt about ride expiry %v, got %v",
			rideExpiresAt,
			*persistedNextAttemptAt,
		)
	}

	if persistedLastAttemptAt == nil {
		t.Fatal(
			"expected last_dispatch_attempt_at to be populated",
		)
	}

	lastAttemptDifference :=
		persistedLastAttemptAt.Sub(
			attemptedAt,
		)

	if lastAttemptDifference < 0 {
		lastAttemptDifference =
			-lastAttemptDifference
	}

	if lastAttemptDifference > timestampTolerance {
		t.Fatalf(
			"expected persisted last attempt about %v, got %v",
			attemptedAt,
			*persistedLastAttemptAt,
		)
	}
}

func TestScheduleDispatchRetryRejectsAlreadyExpiredRide(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	rideRequestID := uuid.NewString()

	now := time.Now().UTC()

	expiresAt := now.Add(
		-1 * time.Second,
	)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO ride_requests
			(
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
				dispatch_retry_count,
				next_dispatch_attempt_at,
				last_dispatch_attempt_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Expired Retry Boundary Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Expired ride must not receive another retry',
				$3,
				$4,
				0,
				NULL,
				NULL,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		expiresAt,
	)
	if err != nil {
		t.Fatalf(
			"create expired retry-boundary ride: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expired retry-boundary ride: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewRideRequestRepository(db)

	_, _, err = repo.ScheduleDispatchRetry(
		ctx,
		rideRequestID,
		now,
	)

	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected repository.ErrNotFound for expired ride, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// Verify rejected scheduling did not mutate retry state.
	// ---------------------------------------------------------

	var (
		status                string
		retryCount            int
		nextDispatchAttemptAt *time.Time
		lastDispatchAttemptAt *time.Time
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				dispatch_retry_count,
				next_dispatch_attempt_at,
				last_dispatch_attempt_at
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&status,
		&retryCount,
		&nextDispatchAttemptAt,
		&lastDispatchAttemptAt,
	)
	if err != nil {
		t.Fatalf(
			"read expired retry-boundary state: %v",
			err,
		)
	}

	if status != "PENDING" {
		t.Fatalf(
			"expected repository scheduling to leave status PENDING, got %s",
			status,
		)
	}

	if retryCount != 0 {
		t.Fatalf(
			"expected retry count 0, got %d",
			retryCount,
		)
	}

	if nextDispatchAttemptAt != nil {
		t.Fatalf(
			"expected next_dispatch_attempt_at NULL, got %v",
			*nextDispatchAttemptAt,
		)
	}

	if lastDispatchAttemptAt != nil {
		t.Fatalf(
			"expected last_dispatch_attempt_at NULL, got %v",
			*lastDispatchAttemptAt,
		)
	}
}
