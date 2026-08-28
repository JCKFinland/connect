package dispatch

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestCreateOfferConcurrentSameRide(t *testing.T) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Load normal CONNECT configuration and database.
	// ---------------------------------------------------------

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

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	// ---------------------------------------------------------
	// 3. Test fixture IDs.
	//
	// John is the existing SEDAN driver used by CONNECT's
	// dispatch integration tests.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	// ---------------------------------------------------------
	// 4. Do not interfere with a legitimate active offer.
	//
	// Stale offers may safely be expired first.
	// ---------------------------------------------------------

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before concurrency test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	// ---------------------------------------------------------
	// 5. Preserve John's current presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 6. Make John eligible for this controlled test.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Create disposable PENDING ride request.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

	if _, err := db.Exec(
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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Concurrency Advisory Lock Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Concurrent CreateOffer integration test',
				$3,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
	); err != nil {
		t.Fatalf(
			"create concurrency test ride request: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. Always remove disposable test data.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup dispatch offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup ride request: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Construct the real dispatch service.
	// ---------------------------------------------------------

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// 10. Launch simultaneous dispatch attempts.
	// ---------------------------------------------------------

	const attemptCount = 5

	start := make(chan struct{})

	errs := make(
		chan error,
		attemptCount,
	)

	var wg sync.WaitGroup

	for i := 0; i < attemptCount; i++ {

		wg.Add(1)

		go func() {
			defer wg.Done()

			// All goroutines wait here so the CreateOffer calls
			// begin as close together as possible.
			<-start

			_, err := service.CreateOffer(
				ctx,
				rideRequestID,
				"",
			)

			errs <- err
		}()
	}

	close(start)

	wg.Wait()
	close(errs)

	// ---------------------------------------------------------
	// 11. Exactly one concurrent dispatch must succeed.
	// ---------------------------------------------------------

	successCount := 0
	failureCount := 0

	for dispatchErr := range errs {

		if dispatchErr == nil {
			successCount++
			continue
		}

		failureCount++

		// Losing requests should normally discover that the first
		// transaction already moved the ride from PENDING to MATCHING.
		//
		// We deliberately do not require an exact error string because
		// the important invariant is database state, not which loser
		// wakes first.
		if errors.Is(
			dispatchErr,
			context.Canceled,
		) {
			t.Fatalf(
				"unexpected context cancellation: %v",
				dispatchErr,
			)
		}
	}

	if successCount != 1 {
		t.Fatalf(
			"expected exactly 1 successful dispatch, got %d successes and %d failures",
			successCount,
			failureCount,
		)
	}

	if failureCount != attemptCount-1 {
		t.Fatalf(
			"expected %d failed concurrent dispatches, got %d",
			attemptCount-1,
			failureCount,
		)
	}

	// ---------------------------------------------------------
	// 12. Verify exactly one offer exists for the ride.
	// ---------------------------------------------------------

	var totalOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
		`,
		rideRequestID,
	).Scan(
		&totalOfferCount,
	); err != nil {
		t.Fatalf(
			"count concurrency test offers: %v",
			err,
		)
	}

	if totalOfferCount != 1 {
		t.Fatalf(
			"expected exactly 1 dispatch offer, got %d",
			totalOfferCount,
		)
	}

	// ---------------------------------------------------------
	// 13. Verify exactly one PENDING offer exists.
	// ---------------------------------------------------------

	var pendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
			  AND status = 'PENDING'
		`,
		rideRequestID,
	).Scan(
		&pendingOfferCount,
	); err != nil {
		t.Fatalf(
			"count pending concurrency test offers: %v",
			err,
		)
	}

	if pendingOfferCount != 1 {
		t.Fatalf(
			"expected exactly 1 PENDING offer, got %d",
			pendingOfferCount,
		)
	}

	// ---------------------------------------------------------
	// 14. Verify ride lifecycle state.
	// ---------------------------------------------------------

	var rideStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&rideStatus,
	); err != nil {
		t.Fatalf(
			"get concurrency test ride status: %v",
			err,
		)
	}

	if rideStatus != rideRequestStatusMatching {
		t.Fatalf(
			"expected ride request status %s, got %s",
			rideRequestStatusMatching,
			rideStatus,
		)
	}
}

func TestAcceptOfferConcurrentSameOffer(t *testing.T) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

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

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	// ---------------------------------------------------------
	// 3. Existing controlled test fixture.
	//
	// dispatch_offers.driver_id uses drivers.id.
	// driver_presence/trips.driver_id use users.id.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	// ---------------------------------------------------------
	// 4. Clear only stale PENDING offers.
	// ---------------------------------------------------------

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale dispatch offers: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 5. Avoid interfering with legitimate active state.
	// ---------------------------------------------------------

	var activePendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(&activePendingOfferCount); err != nil {
		t.Fatalf(
			"check active pending offer: %v",
			err,
		)
	}

	if activePendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	var activeTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				  'COMPLETED',
				  'CANCELLED',
				  'NO_DRIVER_AVAILABLE',
				  'EXPIRED'
			  )
		`,
		johnUserID,
	).Scan(&activeTripCount); err != nil {
		t.Fatalf(
			"check existing active trip: %v",
			err,
		)
	}

	if activeTripCount != 0 {
		t.Skip(
			"John already has an active trip",
		)
	}

	// ---------------------------------------------------------
	// 6. Preserve John's presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 7. Make John AVAILABLE for controlled acceptance.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. Create disposable MATCHING ride request.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	offerID := uuid.NewString()
	now := time.Now().UTC()

	if _, err := db.Exec(
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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Concurrent Acceptance Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'MATCHING',
				'Concurrent AcceptOffer integration test',
				$3,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
	); err != nil {
		t.Fatalf(
			"create acceptance test ride request: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Create one fresh PENDING dispatch offer.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			INSERT INTO dispatch_offers
			(
				id,
				ride_request_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				offered_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				'PENDING',
				$8,
				$9,
				$8,
				$8
			)
		`,
		offerID,
		rideRequestID,
		johnDriverID,
		johnVehicleID,
		companyID,
		branchID,
		fleetID,
		now,
		now.Add(5*time.Minute),
	); err != nil {
		t.Fatalf(
			"create acceptance test dispatch offer: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 10. Cleanup disposable state.
	//
	// trips must be deleted before the offer/ride because of FKs.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup acceptance test trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE id = $1
			`,
			offerID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup acceptance test offer: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup acceptance test ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 11. Construct real dispatch service.
	// ---------------------------------------------------------

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	tripRepo :=
		postgresrepo.NewTripRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Trips:        tripRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// 12. Launch simultaneous acceptance attempts.
	// ---------------------------------------------------------

	const attemptCount = 5

	start := make(chan struct{})
	errs := make(chan error, attemptCount)

	var wg sync.WaitGroup

	for i := 0; i < attemptCount; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			_, err := service.AcceptOffer(
				ctx,
				offerID,
			)

			errs <- err
		}()
	}

	close(start)

	wg.Wait()
	close(errs)

	// ---------------------------------------------------------
	// 13. Exactly one acceptance may succeed.
	// ---------------------------------------------------------

	successCount := 0
	alreadyResolvedCount := 0
	otherFailureCount := 0

	for acceptErr := range errs {
		if acceptErr == nil {
			successCount++
			continue
		}

		if errors.Is(
			acceptErr,
			ErrDispatchOfferAlreadyResolved,
		) {
			alreadyResolvedCount++
			continue
		}

		otherFailureCount++

		t.Logf(
			"unexpected concurrent acceptance error: %v",
			acceptErr,
		)
	}

	if successCount != 1 {
		t.Fatalf(
			"expected exactly 1 successful acceptance, got %d",
			successCount,
		)
	}

	if alreadyResolvedCount != attemptCount-1 {
		t.Fatalf(
			"expected %d already-resolved failures, got %d; other failures=%d",
			attemptCount-1,
			alreadyResolvedCount,
			otherFailureCount,
		)
	}

	// ---------------------------------------------------------
	// 14. Verify exactly one trip exists.
	// ---------------------------------------------------------

	var tripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE ride_request_id = $1
		`,
		rideRequestID,
	).Scan(&tripCount); err != nil {
		t.Fatalf(
			"count accepted trips: %v",
			err,
		)
	}

	if tripCount != 1 {
		t.Fatalf(
			"expected exactly 1 trip, got %d",
			tripCount,
		)
	}

	// ---------------------------------------------------------
	// 15. Verify offer is ACCEPTED exactly once.
	// ---------------------------------------------------------

	var offerStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM dispatch_offers
			WHERE id = $1
		`,
		offerID,
	).Scan(&offerStatus); err != nil {
		t.Fatalf(
			"get accepted offer status: %v",
			err,
		)
	}

	if offerStatus != dispatchOfferStatusAccepted {
		t.Fatalf(
			"expected offer status %s, got %s",
			dispatchOfferStatusAccepted,
			offerStatus,
		)
	}

	// ---------------------------------------------------------
	// 16. Verify ride is ACCEPTED.
	// ---------------------------------------------------------

	var rideStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(&rideStatus); err != nil {
		t.Fatalf(
			"get accepted ride status: %v",
			err,
		)
	}

	if rideStatus != rideRequestStatusAccepted {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusAccepted,
			rideStatus,
		)
	}

	// ---------------------------------------------------------
	// 17. Verify John became BUSY.
	// ---------------------------------------------------------

	var availabilityStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(&availabilityStatus); err != nil {
		t.Fatalf(
			"get accepted driver availability: %v",
			err,
		)
	}

	if availabilityStatus != driverStatusBusy {
		t.Fatalf(
			"expected driver status %s, got %s",
			driverStatusBusy,
			availabilityStatus,
		)
	}
}

func TestDispatchRetryBackoffPersistence(t *testing.T) {
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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Backoff Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Dispatch retry backoff persistence test',
				$3,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create retry backoff test ride: %v",
			err,
		)
	}

	defer func() {
		_, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		)

		if cleanupErr != nil {
			t.Logf(
				"cleanup retry test ride: %v",
				cleanupErr,
			)
		}
	}()

	repo := postgresrepo.NewRideRequestRepository(db)

	expectedDelays := []time.Duration{
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		30 * time.Second,
		30 * time.Second,
	}

	// PostgreSQL TIMESTAMPTZ uses microsecond precision, while Go
	// time.Time can contain nanosecond precision. A small tolerance
	// prevents false test failures caused only by DB precision.
	const timestampTolerance = time.Millisecond

	for i, expectedDelay := range expectedDelays {

		attemptedAt := now.Add(
			time.Duration(i) * time.Minute,
		)

		retryCount, nextAttemptAt, err :=
			repo.ScheduleDispatchRetry(
				ctx,
				rideRequestID,
				attemptedAt,
			)

		if err != nil {
			t.Fatalf(
				"schedule retry %d: %v",
				i+1,
				err,
			)
		}

		expectedRetryCount := i + 1

		// ---------------------------------------------------------
		// Verify returned retry count.
		// ---------------------------------------------------------

		if retryCount != expectedRetryCount {
			t.Fatalf(
				"expected retry count %d, got %d",
				expectedRetryCount,
				retryCount,
			)
		}

		// ---------------------------------------------------------
		// Verify calculated backoff delay.
		// ---------------------------------------------------------

		actualDelay := nextAttemptAt.Sub(
			attemptedAt,
		)

		delayDifference :=
			actualDelay - expectedDelay

		if delayDifference < 0 {
			delayDifference = -delayDifference
		}

		if delayDifference > timestampTolerance {
			t.Fatalf(
				"retry %d: expected delay about %s, got %s",
				expectedRetryCount,
				expectedDelay,
				actualDelay,
			)
		}

		// ---------------------------------------------------------
		// Load persisted retry state directly from PostgreSQL.
		// ---------------------------------------------------------

		var (
			persistedRetryCount    int
			persistedNextAttemptAt *time.Time
			persistedLastAttemptAt *time.Time
		)

		err = db.QueryRow(
			ctx,
			`
				SELECT
					dispatch_retry_count,
					next_dispatch_attempt_at,
					last_dispatch_attempt_at
				FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		).Scan(
			&persistedRetryCount,
			&persistedNextAttemptAt,
			&persistedLastAttemptAt,
		)

		if err != nil {
			t.Fatalf(
				"read persisted retry state: %v",
				err,
			)
		}

		// ---------------------------------------------------------
		// Verify persisted retry count.
		// ---------------------------------------------------------

		if persistedRetryCount != expectedRetryCount {
			t.Fatalf(
				"expected persisted retry count %d, got %d",
				expectedRetryCount,
				persistedRetryCount,
			)
		}

		if persistedNextAttemptAt == nil {
			t.Fatal(
				"expected next_dispatch_attempt_at to be populated",
			)
		}

		if persistedLastAttemptAt == nil {
			t.Fatal(
				"expected last_dispatch_attempt_at to be populated",
			)
		}

		// ---------------------------------------------------------
		// Verify persisted next-attempt timestamp.
		//
		// Compare instants using a tolerance rather than requiring
		// identical timezone representations or nanosecond precision.
		// ---------------------------------------------------------

		nextAttemptDifference :=
			persistedNextAttemptAt.Sub(
				nextAttemptAt,
			)

		if nextAttemptDifference < 0 {
			nextAttemptDifference =
				-nextAttemptDifference
		}

		if nextAttemptDifference >
			timestampTolerance {

			t.Fatalf(
				"expected persisted next attempt about %v, got %v",
				nextAttemptAt,
				*persistedNextAttemptAt,
			)
		}

		// ---------------------------------------------------------
		// Verify persisted last-attempt timestamp.
		//
		// UTC and EEST representations may differ visually but can
		// represent the same instant. Precision tolerance handles
		// PostgreSQL's microsecond storage as well.
		// ---------------------------------------------------------

		lastAttemptDifference :=
			persistedLastAttemptAt.Sub(
				attemptedAt,
			)

		if lastAttemptDifference < 0 {
			lastAttemptDifference =
				-lastAttemptDifference
		}

		if lastAttemptDifference >
			timestampTolerance {

			t.Fatalf(
				"expected persisted last attempt about %v, got %v",
				attemptedAt,
				*persistedLastAttemptAt,
			)
		}
	}
}

func TestCreateOfferResetsDispatchRetryState(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before reset test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(&existingPendingOfferCount); err != nil {
		t.Fatalf(
			"check existing pending offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare driver presence: %v",
			err,
		)
	}

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				'Retry Reset Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'CreateOffer retry reset integration test',
				$3,
				4,
				$4,
				$5,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(20*time.Second),
		now.Add(-5*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"create retry reset test ride: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		)

		_, _ = db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		)
	}()

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)
	if err != nil {
		t.Fatalf(
			"create offer with retry reset: %v",
			err,
		)
	}

	if offer == nil {
		t.Fatal(
			"expected created dispatch offer",
		)
	}

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
			"read reset retry state: %v",
			err,
		)
	}

	if status != rideRequestStatusMatching {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusMatching,
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

func TestCreateOfferRejectsExpiredRideAndPersistsExpiredState(t *testing.T) {
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
	expiredAt := now.Add(-1 * time.Minute)

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
				'Expired Ride Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Expired ride CreateOffer lifecycle test',
				$3,
				$4,
				4,
				$5,
				$6,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		expiredAt,
		now.Add(20*time.Second),
		now.Add(-5*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"create expired ride request: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expired ride offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expired ride request: %v",
				cleanupErr,
			)
		}
	}()

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)

	if !errors.Is(
		err,
		ErrRideRequestExpired,
	) {
		t.Fatalf(
			"expected ErrRideRequestExpired, got offer=%v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf(
			"expected no dispatch offer for expired ride, got %+v",
			offer,
		)
	}

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
			"read expired ride lifecycle state: %v",
			err,
		)
	}

	if status != rideRequestStatusExpired {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusExpired,
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

	var offerCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
		`,
		rideRequestID,
	).Scan(
		&offerCount,
	)
	if err != nil {
		t.Fatalf(
			"count expired ride dispatch offers: %v",
			err,
		)
	}

	if offerCount != 0 {
		t.Fatalf(
			"expected zero dispatch offers for expired ride, got %d",
			offerCount,
		)
	}
}

func TestCreateOfferCapsOfferExpiryAtRideExpiry(t *testing.T) {
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

	// This test temporarily uses John's dispatch fixture.
	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// Expire only genuinely stale offers before checking whether
	// the controlled fixture is free.
	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before expiry-cap test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	// Preserve the existing presence state.
	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// Make John eligible for the controlled dispatch.
	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare driver presence: %v",
			err,
		)
	}

	rideRequestID := uuid.NewString()

	now := time.Now().UTC()

	// Deliberately shorter than the normal 30-second dispatch
	// offer timeout.
	rideExpiresAt := now.Add(
		10 * time.Second,
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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Offer Expiry Cap Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Dispatch offer hard-expiry cap integration test',
				$3,
				$4,
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
			"create offer-expiry-cap ride: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expiry-cap offer: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expiry-cap ride: %v",
				cleanupErr,
			)
		}
	}()

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)
	if err != nil {
		t.Fatalf(
			"create expiry-capped dispatch offer: %v",
			err,
		)
	}

	if offer == nil {
		t.Fatal(
			"expected created dispatch offer",
		)
	}

	// ---------------------------------------------------------
	// The offer may not extend beyond the ride's hard expiry.
	// ---------------------------------------------------------

	if offer.ExpiresAt.After(rideExpiresAt) {
		t.Fatalf(
			"offer expiry %v exceeds ride expiry %v",
			offer.ExpiresAt,
			rideExpiresAt,
		)
	}

	// PostgreSQL may round TIMESTAMPTZ to microsecond precision.
	const timestampTolerance = time.Millisecond

	expiryDifference :=
		offer.ExpiresAt.Sub(rideExpiresAt)

	if expiryDifference < 0 {
		expiryDifference = -expiryDifference
	}

	if expiryDifference > timestampTolerance {
		t.Fatalf(
			"expected offer expiry about %v, got %v",
			rideExpiresAt,
			offer.ExpiresAt,
		)
	}

	// Verify the persisted database value too.
	var persistedOfferExpiresAt time.Time

	err = db.QueryRow(
		ctx,
		`
			SELECT expires_at
			FROM dispatch_offers
			WHERE id = $1
		`,
		offer.ID,
	).Scan(
		&persistedOfferExpiresAt,
	)
	if err != nil {
		t.Fatalf(
			"read persisted offer expiry: %v",
			err,
		)
	}

	if persistedOfferExpiresAt.After(
		rideExpiresAt.Add(timestampTolerance),
	) {
		t.Fatalf(
			"persisted offer expiry %v exceeds ride expiry %v",
			persistedOfferExpiresAt,
			rideExpiresAt,
		)
	}

	persistedDifference :=
		persistedOfferExpiresAt.Sub(
			rideExpiresAt,
		)

	if persistedDifference < 0 {
		persistedDifference = -persistedDifference
	}

	if persistedDifference > timestampTolerance {
		t.Fatalf(
			"expected persisted offer expiry about %v, got %v",
			rideExpiresAt,
			persistedOfferExpiresAt,
		)
	}
}

func TestCreateOfferRejectsDriverWithStaleHeartbeat(t *testing.T) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatalf(
			"heartbeat timeout must be greater than zero, got %v",
			cfg.Presence.HeartbeatTimeout,
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

	// ---------------------------------------------------------
	// 3. Serialize access to John's shared dispatch fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// ---------------------------------------------------------
	// 4. Remove only genuinely expired pending offers before
	//    checking whether John's controlled fixture is free.
	// ---------------------------------------------------------

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before heartbeat test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	// ---------------------------------------------------------
	// 5. Preserve John's complete relevant presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					latitude = $4,
					longitude = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 6. Make John look AVAILABLE but deliberately stale.
	//
	// Everything except heartbeat freshness is valid.
	// ---------------------------------------------------------

	staleHeartbeat :=
		time.Now().UTC().Add(
			-(cfg.Presence.HeartbeatTimeout + time.Minute),
		)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = $2,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
		staleHeartbeat,
	); err != nil {
		t.Fatalf(
			"prepare stale-heartbeat driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Create disposable PENDING ride request.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Stale Heartbeat Dispatch Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Driver heartbeat freshness regression test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create stale-heartbeat test ride: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. Always remove disposable dispatch state.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup stale-heartbeat offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup stale-heartbeat ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Construct the real dispatch service.
	// ---------------------------------------------------------

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// 10. Attempt dispatch.
	//
	// Another eligible driver may legitimately receive the ride,
	// so ErrNoAvailableDrivers and successful dispatch are both
	// acceptable outcomes.
	//
	// The invariant is that stale John must never receive it.
	// ---------------------------------------------------------

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)

	if err != nil &&
		!errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {

		t.Fatalf(
			"unexpected CreateOffer error: %v",
			err,
		)
	}

	if offer != nil &&
		offer.DriverID == johnDriverID {

		t.Fatalf(
			"stale driver %s received dispatch offer %s",
			johnDriverID,
			offer.ID,
		)
	}

	// ---------------------------------------------------------
	// 11. Verify persistence also contains no offer for John.
	// ---------------------------------------------------------

	var johnOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
			  AND driver_id = $2
		`,
		rideRequestID,
		johnDriverID,
	).Scan(
		&johnOfferCount,
	); err != nil {
		t.Fatalf(
			"count stale-driver offers: %v",
			err,
		)
	}

	if johnOfferCount != 0 {
		t.Fatalf(
			"expected zero offers for stale driver, got %d",
			johnOfferCount,
		)
	}

	// ---------------------------------------------------------
	// 12. Verify John's stale presence was not mutated merely
	//     because dispatch rejected him.
	// ---------------------------------------------------------

	var (
		persistedIsOnline  bool
		persistedStatus    string
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedIsOnline,
		&persistedStatus,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read stale driver presence after dispatch: %v",
			err,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected stale driver presence to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected stale driver to remain AVAILABLE, got %s",
			persistedStatus,
		)
	}

	if persistedHeartbeat == nil {
		t.Fatal(
			"expected stale heartbeat timestamp to remain present",
		)
	}

	if persistedHeartbeat.After(
		time.Now().UTC().Add(
			-cfg.Presence.HeartbeatTimeout,
		),
	) {
		t.Fatalf(
			"expected heartbeat to remain stale, got %v",
			*persistedHeartbeat,
		)
	}
}

func TestCreateOfferRejectsDriverWithFutureHeartbeat(t *testing.T) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatalf(
			"heartbeat timeout must be greater than zero, got %v",
			cfg.Presence.HeartbeatTimeout,
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

	// ---------------------------------------------------------
	// 3. Serialize access to John's shared dispatch fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// ---------------------------------------------------------
	// 4. Remove only genuinely expired pending offers before
	//    checking whether John's controlled fixture is free.
	// ---------------------------------------------------------

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before heartbeat test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	// ---------------------------------------------------------
	// 5. Preserve John's complete relevant presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					latitude = $4,
					longitude = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// 6. Make John look AVAILABLE but deliberately future-dated.
	//
	// Everything except heartbeat timestamp validity is correct.
	// Dispatch must reject heartbeats that appear to come from
	// the future.

	futureHeartbeat :=
		time.Now().UTC().Add(
			5 * time.Minute,
		)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = $2,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
		futureHeartbeat,
	); err != nil {
		t.Fatalf(
			"prepare future-heartbeat driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Create disposable PENDING ride request.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Future Heartbeat Dispatch Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Future-dated driver heartbeat regression test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create future-heartbeat test ride: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. Always remove disposable dispatch state.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup future-heartbeat offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup future-heartbeat ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Construct the real dispatch service.
	// ---------------------------------------------------------

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// 10. Attempt dispatch.
	//
	// Another eligible driver may legitimately receive the ride,
	// so ErrNoAvailableDrivers and successful dispatch are both
	// acceptable outcomes.
	//
	// The invariant is that stale John must never receive it.
	// ---------------------------------------------------------

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)

	if err != nil &&
		!errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {

		t.Fatalf(
			"unexpected CreateOffer error: %v",
			err,
		)
	}

	if offer != nil &&
		offer.DriverID == johnDriverID {

		t.Fatalf(
			"future-dated driver %s received dispatch offer %s",
			johnDriverID,
			offer.ID,
		)
	}

	// ---------------------------------------------------------
	// 11. Verify persistence also contains no offer for John.
	// ---------------------------------------------------------

	var johnOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
			  AND driver_id = $2
		`,
		rideRequestID,
		johnDriverID,
	).Scan(
		&johnOfferCount,
	); err != nil {
		t.Fatalf(
			"count stale-driver offers: %v",
			err,
		)
	}

	if johnOfferCount != 0 {
		t.Fatalf(
			"expected zero offers for future-dated driver, got %d",
			johnOfferCount,
		)
	}

	// ---------------------------------------------------------
	// 12. Verify John's stale presence was not mutated merely
	//     because dispatch rejected him.
	// ---------------------------------------------------------

	var (
		persistedIsOnline  bool
		persistedStatus    string
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedIsOnline,
		&persistedStatus,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read stale driver presence after dispatch: %v",
			err,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected future-dated driver presence to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected future-dated driver to remain AVAILABLE, got %s",
			persistedStatus,
		)
	}
	if persistedHeartbeat == nil {
		t.Fatal(
			"expected future heartbeat timestamp to remain present",
		)
	}

	if !persistedHeartbeat.After(time.Now().UTC()) {
		t.Fatalf(
			"expected heartbeat to remain future-dated, got %v",
			*persistedHeartbeat,
		)
	}
}

func TestCreateOfferRejectsDriverWithMissingHeartbeat(t *testing.T) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatalf(
			"heartbeat timeout must be greater than zero, got %v",
			cfg.Presence.HeartbeatTimeout,
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

	// ---------------------------------------------------------
	// 3. Serialize access to John's shared dispatch fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// ---------------------------------------------------------
	// 4. Remove only genuinely expired pending offers before
	//    checking whether John's controlled fixture is free.
	// ---------------------------------------------------------

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before heartbeat test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	// ---------------------------------------------------------
	// 5. Preserve John's complete relevant presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					latitude = $4,
					longitude = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()
	// ---------------------------------------------------------
	// 6. Make John look AVAILABLE but without a heartbeat.
	//
	// Everything except heartbeat presence is valid.
	// A driver with no heartbeat timestamp is not dispatchable.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = NULL,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare missing-heartbeat driver presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Create disposable PENDING ride request.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Missing Heartbeat Dispatch Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Missing driver heartbeat regression test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create missing-heartbeat test ride: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. Always remove disposable dispatch state.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup missing-heartbeat offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup missing-heartbeat ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Construct the real dispatch service.
	// ---------------------------------------------------------

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverAssignmentRepo :=
		postgresrepo.NewDriverAssignmentRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	vehicleRepo :=
		postgresrepo.NewVehicleRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// 10. Attempt dispatch.
	//
	// Another eligible driver may legitimately receive the ride,
	// so ErrNoAvailableDrivers and successful dispatch are both
	// acceptable outcomes.
	//
	// The invariant is that stale John must never receive it.
	// ---------------------------------------------------------

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)

	if err != nil &&
		!errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {

		t.Fatalf(
			"unexpected CreateOffer error: %v",
			err,
		)
	}

	if offer != nil &&
		offer.DriverID == johnDriverID {

		t.Fatalf(
			"driver without heartbeat %s received dispatch offer %s",
			johnDriverID,
			offer.ID,
		)
	}

	// ---------------------------------------------------------
	// 11. Verify persistence also contains no offer for John.
	// ---------------------------------------------------------

	var johnOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
			  AND driver_id = $2
		`,
		rideRequestID,
		johnDriverID,
	).Scan(
		&johnOfferCount,
	); err != nil {
		t.Fatalf(
			"count stale-driver offers: %v",
			err,
		)
	}

	if johnOfferCount != 0 {
		t.Fatalf(
			"expected zero offers for driver without heartbeat, got %d",
			johnOfferCount,
		)
	}

	// ---------------------------------------------------------
	// 12. Verify John's stale presence was not mutated merely
	//     because dispatch rejected him.
	// ---------------------------------------------------------

	var (
		persistedIsOnline  bool
		persistedStatus    string
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedIsOnline,
		&persistedStatus,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read missing-heartbeat driver presence after dispatch: %v",
			err,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected driver without heartbeat to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected driver without heartbeat to remain AVAILABLE, got %s",
			persistedStatus,
		)
	}

	if persistedHeartbeat != nil {
		t.Fatalf(
			"expected heartbeat to remain NULL, got %v",
			*persistedHeartbeat,
		)
	}
}

func TestDispatchRideRejectsDriverWithStaleHeartbeat(t *testing.T) {
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

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatalf(
			"heartbeat timeout must be greater than zero, got %v",
			cfg.Presence.HeartbeatTimeout,
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

	// ---------------------------------------------------------
	// Serialize access to John's shared dispatch fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"
	)

	// ---------------------------------------------------------
	// Make sure John is not already committed to an active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				'COMPLETED',
				'CANCELLED',
				'NO_DRIVER_AVAILABLE',
				'EXPIRED'
			  )
		`,
		johnUserID,
	).Scan(
		&existingActiveTripCount,
	); err != nil {
		t.Fatalf(
			"check existing active John trip: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Skip(
			"John already has an active trip",
		)
	}

	// ---------------------------------------------------------
	// Preserve John's presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original John presence: %v",
			err,
		)
	}

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					latitude = $4,
					longitude = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore John presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Make John otherwise eligible but with a stale heartbeat.
	// ---------------------------------------------------------

	staleHeartbeat :=
		time.Now().UTC().Add(
			-(cfg.Presence.HeartbeatTimeout + time.Minute),
		)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = $2,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
		staleHeartbeat,
	); err != nil {
		t.Fatalf(
			"prepare stale John presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// Create disposable PENDING ride.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'DispatchRide Stale Heartbeat Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'DispatchRide stale heartbeat regression test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create DispatchRide stale-heartbeat ride: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide stale-heartbeat trips: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide stale-heartbeat ride: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Construct the real dispatch service.
	//
	// DispatchRide builds transaction-scoped repositories
	// internally, so DB + Config are sufficient.
	// ---------------------------------------------------------

	service := NewService(
		Dependencies{
			DB:     db,
			Config: cfg,
		},
	)

	// ---------------------------------------------------------
	// Attempt direct dispatch.
	//
	// Another legitimate driver may be selected, so successful
	// dispatch and ErrNoAvailableDrivers are both acceptable.
	//
	// John specifically must never be selected.
	// ---------------------------------------------------------

	trip, err := service.DispatchRide(
		ctx,
		rideRequestID,
	)

	if err != nil &&
		!errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {

		t.Fatalf(
			"unexpected DispatchRide error: %v",
			err,
		)
	}

	if trip != nil &&
		trip.DriverID == johnUserID {

		t.Fatalf(
			"stale John received dispatched trip %s",
			trip.ID,
		)
	}

	// ---------------------------------------------------------
	// Verify there is no persisted trip assigned to John.
	//
	// trips.driver_id uses the user ID in this lifecycle.
	// ---------------------------------------------------------

	var johnTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE ride_request_id = $1
			  AND driver_id = $2
		`,
		rideRequestID,
		johnUserID,
	).Scan(
		&johnTripCount,
	); err != nil {
		t.Fatalf(
			"count stale John dispatched trips: %v",
			err,
		)
	}

	if johnTripCount != 0 {
		t.Fatalf(
			"expected zero trips for stale John, got %d",
			johnTripCount,
		)
	}

	// ---------------------------------------------------------
	// John's presence must not be mutated by rejecting him.
	// ---------------------------------------------------------

	var (
		persistedIsOnline  bool
		persistedStatus    string
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedIsOnline,
		&persistedStatus,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read John presence after stale DispatchRide: %v",
			err,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected stale John to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected stale John to remain AVAILABLE, got %s",
			persistedStatus,
		)
	}

	if persistedHeartbeat == nil {
		t.Fatal(
			"expected stale heartbeat to remain present",
		)
	}

	if persistedHeartbeat.After(
		time.Now().UTC().Add(
			-cfg.Presence.HeartbeatTimeout,
		),
	) {
		t.Fatalf(
			"expected John's heartbeat to remain stale, got %v",
			*persistedHeartbeat,
		)
	}

	// ---------------------------------------------------------
	// If no other driver was selected, the ride must still be
	// PENDING. If another driver legitimately won dispatch, the
	// ride may correctly be ACCEPTED.
	// ---------------------------------------------------------

	var persistedRideStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&persistedRideStatus,
	); err != nil {
		t.Fatalf(
			"read ride after stale DispatchRide: %v",
			err,
		)
	}

	if trip == nil &&
		persistedRideStatus != rideRequestStatusPending {

		t.Fatalf(
			"expected ride to remain %s when no driver was dispatched, got %s",
			rideRequestStatusPending,
			persistedRideStatus,
		)
	}

	if trip != nil &&
		persistedRideStatus != rideRequestStatusAccepted {

		t.Fatalf(
			"expected ride to be %s after another driver dispatch, got %s",
			rideRequestStatusAccepted,
			persistedRideStatus,
		)
	}

	_ = johnDriverID
}

func TestRejectOfferDoesNotResurrectExpiredRide(t *testing.T) {
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

	// This test uses John's operational driver fixture because
	// dispatch_offers.driver_id has a foreign-key relationship
	// to drivers.id.
	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// Remove only genuinely stale offers before checking whether
	// the shared fixture is currently available.
	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before rejection lifecycle test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	rideRequestID := uuid.NewString()
	offerID := uuid.NewString()

	now := time.Now().UTC()

	// The ride itself has already expired.
	rideExpiresAt := now.Add(
		-1 * time.Minute,
	)

	// The dispatch offer is deliberately still valid.
	//
	// This proves that ride-request hard expiry independently
	// terminates the ride lifecycle even when the driver rejects
	// a still-valid offer.
	offerExpiresAt := now.Add(
		5 * time.Minute,
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
				'Expired Rejection Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'MATCHING',
				'RejectOffer hard-expiry lifecycle test',
				$3,
				$4,
				4,
				$5,
				$6,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		rideExpiresAt,
		now.Add(20*time.Second),
		now.Add(-5*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"create expired MATCHING ride: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO dispatch_offers
			(
				id,
				ride_request_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				offered_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				'PENDING',
				$8,
				$9,
				$8,
				$8
			)
		`,
		offerID,
		rideRequestID,
		johnDriverID,
		johnVehicleID,
		companyID,
		branchID,
		fleetID,
		now,
		offerExpiresAt,
	)
	if err != nil {
		t.Fatalf(
			"create rejection lifecycle dispatch offer: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE id = $1
			`,
			offerID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup rejection lifecycle offer: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup rejection lifecycle ride: %v",
				cleanupErr,
			)
		}
	}()

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Offers:       offerRepo,
		},
	)

	resolvedOffer, err := service.RejectOffer(
		ctx,
		offerID,
		"Driver declined expired ride",
	)
	if err != nil {
		t.Fatalf(
			"reject expired ride dispatch offer: %v",
			err,
		)
	}

	if resolvedOffer == nil {
		t.Fatal(
			"expected resolved dispatch offer",
		)
	}

	if resolvedOffer.Status !=
		dispatchOfferStatusRejected {

		t.Fatalf(
			"expected offer status %s, got %s",
			dispatchOfferStatusRejected,
			resolvedOffer.Status,
		)
	}

	var (
		rideStatus            string
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
		&rideStatus,
		&retryCount,
		&nextDispatchAttemptAt,
		&lastDispatchAttemptAt,
	)
	if err != nil {
		t.Fatalf(
			"read rejected expired ride state: %v",
			err,
		)
	}

	if rideStatus != rideRequestStatusExpired {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusExpired,
			rideStatus,
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

func TestAcceptOfferExpiredRidePersistsTerminalState(t *testing.T) {
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

	// ---------------------------------------------------------
	// Serialize access to John's shared integration fixture.
	// ---------------------------------------------------------

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// ---------------------------------------------------------
	// Remove only genuinely stale offers before checking whether
	// the fixture is free.
	// ---------------------------------------------------------

	if _, err := offerRepo.ExpireStalePending(
		ctx,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"expire stale offers before expired-acceptance test: %v",
			err,
		)
	}

	var existingPendingOfferCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE driver_id = $1
			  AND status = 'PENDING'
		`,
		johnDriverID,
	).Scan(
		&existingPendingOfferCount,
	); err != nil {
		t.Fatalf(
			"check existing pending driver offer: %v",
			err,
		)
	}

	if existingPendingOfferCount != 0 {
		t.Skip(
			"John already has an active PENDING dispatch offer",
		)
	}

	// ---------------------------------------------------------
	// Ensure the fixture is not already involved in an active trip.
	// ---------------------------------------------------------

	var activeTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				  'COMPLETED',
				  'CANCELLED',
				  'NO_DRIVER_AVAILABLE',
				  'EXPIRED'
			  )
		`,
		johnUserID,
	).Scan(
		&activeTripCount,
	); err != nil {
		t.Fatalf(
			"check existing active trip: %v",
			err,
		)
	}

	if activeTripCount != 0 {
		t.Skip(
			"John already has an active trip",
		)
	}

	// ---------------------------------------------------------
	// Preserve John's presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original driver presence: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					last_heartbeat_at = $4,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalHeartbeat,
		)

		if restoreErr != nil {
			t.Logf(
				"restore driver presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Make John AVAILABLE before the acceptance attempt.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				last_heartbeat_at = NOW(),
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare driver presence: %v",
			err,
		)
	}

	rideRequestID := uuid.NewString()
	offerID := uuid.NewString()

	now := time.Now().UTC()

	rideExpiresAt := now.Add(
		-1 * time.Minute,
	)

	// Deliberately keep the offer itself valid.
	//
	// This proves the ride-request hard expiry independently
	// terminates acceptance even when the offer has not yet timed out.
	offerExpiresAt := now.Add(
		5 * time.Minute,
	)

	// ---------------------------------------------------------
	// Create expired MATCHING ride with existing retry state.
	// ---------------------------------------------------------

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
				'Expired Acceptance Test Pickup',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'MATCHING',
				'AcceptOffer hard-expiry lifecycle test',
				$3,
				$4,
				4,
				$5,
				$6,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		rideExpiresAt,
		now.Add(20*time.Second),
		now.Add(-5*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"create expired acceptance test ride: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// Create still-valid PENDING offer.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO dispatch_offers
			(
				id,
				ride_request_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				offered_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				'PENDING',
				$8,
				$9,
				$8,
				$8
			)
		`,
		offerID,
		rideRequestID,
		johnDriverID,
		johnVehicleID,
		companyID,
		branchID,
		fleetID,
		now,
		offerExpiresAt,
	)
	if err != nil {
		t.Fatalf(
			"create expired acceptance test offer: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expired acceptance trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE id = $1
			`,
			offerID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expired acceptance offer: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup expired acceptance ride: %v",
				cleanupErr,
			)
		}
	}()

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	driverPresenceRepo :=
		postgresrepo.NewDriverPresenceRepository(db)

	tripRepo :=
		postgresrepo.NewTripRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Presence:     driverPresenceRepo,
			Trips:        tripRepo,
			Drivers:      driverRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// Attempt acceptance.
	// ---------------------------------------------------------

	trip, err := service.AcceptOffer(
		ctx,
		offerID,
	)

	if !errors.Is(
		err,
		ErrRideRequestExpired,
	) {
		t.Fatalf(
			"expected ErrRideRequestExpired, got trip=%v err=%v",
			trip,
			err,
		)
	}

	if trip != nil {
		t.Fatalf(
			"expected no trip for expired ride, got %+v",
			trip,
		)
	}

	// ---------------------------------------------------------
	// Verify ride terminal state and retry cleanup.
	// ---------------------------------------------------------

	var (
		rideStatus            string
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
		&rideStatus,
		&retryCount,
		&nextDispatchAttemptAt,
		&lastDispatchAttemptAt,
	)
	if err != nil {
		t.Fatalf(
			"read expired acceptance ride state: %v",
			err,
		)
	}

	if rideStatus != rideRequestStatusExpired {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusExpired,
			rideStatus,
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

	// ---------------------------------------------------------
	// Verify dispatch offer was terminally expired.
	// ---------------------------------------------------------

	var offerStatus string

	err = db.QueryRow(
		ctx,
		`
			SELECT status
			FROM dispatch_offers
			WHERE id = $1
		`,
		offerID,
	).Scan(
		&offerStatus,
	)
	if err != nil {
		t.Fatalf(
			"read expired acceptance offer status: %v",
			err,
		)
	}

	if offerStatus != dispatchOfferStatusExpired {
		t.Fatalf(
			"expected offer status %s, got %s",
			dispatchOfferStatusExpired,
			offerStatus,
		)
	}

	// ---------------------------------------------------------
	// Verify no trip was created.
	// ---------------------------------------------------------

	var tripCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE ride_request_id = $1
		`,
		rideRequestID,
	).Scan(
		&tripCount,
	)
	if err != nil {
		t.Fatalf(
			"count expired acceptance trips: %v",
			err,
		)
	}

	if tripCount != 0 {
		t.Fatalf(
			"expected zero trips for expired ride, got %d",
			tripCount,
		)
	}

	// ---------------------------------------------------------
	// Driver must remain AVAILABLE.
	// ---------------------------------------------------------

	var availabilityStatus string

	err = db.QueryRow(
		ctx,
		`
			SELECT availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&availabilityStatus,
	)
	if err != nil {
		t.Fatalf(
			"read driver availability after expired acceptance: %v",
			err,
		)
	}

	if availabilityStatus != "AVAILABLE" {
		t.Fatalf(
			"expected driver to remain AVAILABLE, got %s",
			availabilityStatus,
		)
	}
}

func TestRedispatchWorkerExpiresRideWaitingInBackoff(t *testing.T) {
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
				'Worker Hard Expiry Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Redispatch worker hard-expiry lifecycle test',
				$3,
				$4,
				5,
				$5,
				$6,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(-1*time.Minute),
		now.Add(30*time.Second),
		now.Add(-10*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"create expired backoff ride: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup worker expiry offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup worker expiry ride: %v",
				cleanupErr,
			)
		}
	}()

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Offers:       offerRepo,
		},
	)

	if err := service.runRedispatchCycle(
		ctx,
		100,
	); err != nil {
		t.Fatalf(
			"run redispatch worker cycle: %v",
			err,
		)
	}

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
			"read worker-expired ride state: %v",
			err,
		)
	}

	if status != rideRequestStatusExpired {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusExpired,
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

	var offerCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM dispatch_offers
			WHERE ride_request_id = $1
		`,
		rideRequestID,
	).Scan(
		&offerCount,
	)
	if err != nil {
		t.Fatalf(
			"count worker-expired ride offers: %v",
			err,
		)
	}

	if offerCount != 0 {
		t.Fatalf(
			"expected zero dispatch offers after hard expiry, got %d",
			offerCount,
		)
	}
}

func TestRedispatchDiscoveryCannotDispatchRideExpiredAfterDiscovery(t *testing.T) {
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

	// Historical dispatch activity uses John's real driver/vehicle
	// fixture. Serialize access with the rest of the dispatch tests.
	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnDriverID = "39175f42-0c89-4d45-96be-ed5367506e36"

		johnVehicleID = "6dce24b5-b257-447a-99e0-ef439fbd0e17"

		companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

		branchID = "186f7570-6902-41a2-a1f9-d509a4d90fcb"

		fleetID = "dc46fc5c-7290-462c-a423-22b3c46b7c99"
	)

	rideRequestID := uuid.NewString()
	historicalOfferID := uuid.NewString()

	now := time.Now().UTC()

	// Initially valid long enough to be discovered.
	initialRideExpiry := now.Add(
		5 * time.Minute,
	)

	// ---------------------------------------------------------
	// Create redispatchable PENDING ride.
	// ---------------------------------------------------------

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
				'Redispatch Discovery Race Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Ride expires after redispatch discovery',
				$3,
				$4,
				3,
				NULL,
				$5,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		initialRideExpiry,
		now.Add(-30*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"create redispatch discovery race ride: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// Create historical REJECTED offer.
	//
	// This makes the ride eligible for redispatch discovery while
	// leaving no active PENDING offer.
	// ---------------------------------------------------------

	historicalOfferedAt := now.Add(
		-2 * time.Minute,
	)

	historicalExpiresAt := historicalOfferedAt.Add(
		30 * time.Second,
	)

	historicalRespondedAt := historicalOfferedAt.Add(
		10 * time.Second,
	)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO dispatch_offers
			(
				id,
				ride_request_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				offered_at,
				expires_at,
				responded_at,
				rejection_reason,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				'REJECTED',
				$8,
				$9,
				$10,
				'Historical redispatch race fixture',
				$8,
				$10
			)
		`,
		historicalOfferID,
		rideRequestID,
		johnDriverID,
		johnVehicleID,
		companyID,
		branchID,
		fleetID,
		historicalOfferedAt,
		historicalExpiresAt,
		historicalRespondedAt,
	)
	if err != nil {
		t.Fatalf(
			"create historical dispatch offer: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM dispatch_offers
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup redispatch race offers: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup redispatch race ride: %v",
				cleanupErr,
			)
		}
	}()

	offerRepo :=
		postgresrepo.NewDispatchOfferRepository(db)

	// ---------------------------------------------------------
	// Phase 1: worker discovery sees the ride as redispatchable.
	// ---------------------------------------------------------

	redispatchableIDs, err :=
		offerRepo.ListRedispatchableRideRequestIDs(
			ctx,
			100,
		)
	if err != nil {
		t.Fatalf(
			"discover redispatchable rides: %v",
			err,
		)
	}

	discovered := false

	for _, id := range redispatchableIDs {
		if id == rideRequestID {
			discovered = true
			break
		}
	}

	if !discovered {
		t.Fatalf(
			"expected ride %s to be discovered for redispatch",
			rideRequestID,
		)
	}

	// ---------------------------------------------------------
	// Phase 2: the ride reaches its hard expiry after discovery.
	//
	// This simulates the race between discovery and CreateOffer().
	// ---------------------------------------------------------

	expiredAt := time.Now().UTC().Add(
		-1 * time.Second,
	)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE ride_requests
			SET
				expires_at = $2,
				updated_at = NOW()
			WHERE id = $1
		`,
		rideRequestID,
		expiredAt,
	); err != nil {
		t.Fatalf(
			"expire ride after redispatch discovery: %v",
			err,
		)
	}

	rideRequestRepo :=
		postgresrepo.NewRideRequestRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Config:       cfg,
			RideRequests: rideRequestRepo,
			Offers:       offerRepo,
		},
	)

	// ---------------------------------------------------------
	// Phase 3: stale discovery result attempts dispatch.
	//
	// CreateOffer() must re-check the authoritative ride state
	// after acquiring its database locks.
	// ---------------------------------------------------------

	offer, err := service.CreateOffer(
		ctx,
		rideRequestID,
		"",
	)

	if !errors.Is(
		err,
		ErrRideRequestExpired,
	) {
		t.Fatalf(
			"expected ErrRideRequestExpired after stale discovery, got offer=%v err=%v",
			offer,
			err,
		)
	}

	if offer != nil {
		t.Fatalf(
			"expected no new offer for expired ride, got %+v",
			offer,
		)
	}

	// ---------------------------------------------------------
	// Verify terminal lifecycle state.
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
			"read redispatch race ride state: %v",
			err,
		)
	}

	if status != rideRequestStatusExpired {
		t.Fatalf(
			"expected ride status %s, got %s",
			rideRequestStatusExpired,
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

	// ---------------------------------------------------------
	// Historical offer must remain the only offer for this ride.
	// ---------------------------------------------------------

	var (
		totalOfferCount   int
		pendingOfferCount int
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				COUNT(*),
				COUNT(*) FILTER (
					WHERE status = 'PENDING'
				)
			FROM dispatch_offers
			WHERE ride_request_id = $1
		`,
		rideRequestID,
	).Scan(
		&totalOfferCount,
		&pendingOfferCount,
	)
	if err != nil {
		t.Fatalf(
			"count redispatch race offers: %v",
			err,
		)
	}

	if totalOfferCount != 1 {
		t.Fatalf(
			"expected only historical offer to remain, got %d offers",
			totalOfferCount,
		)
	}

	if pendingOfferCount != 0 {
		t.Fatalf(
			"expected zero PENDING offers after hard expiry, got %d",
			pendingOfferCount,
		)
	}
}

func TestDispatchRideRejectsDriverWithFutureHeartbeat(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	if cfg.Presence.HeartbeatTimeout <= 0 {
		t.Fatalf(
			"heartbeat timeout must be greater than zero, got %v",
			cfg.Presence.HeartbeatTimeout,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"
		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	var existingActiveTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				'COMPLETED',
				'CANCELLED',
				'NO_DRIVER_AVAILABLE',
				'EXPIRED'
			  )
		`,
		johnUserID,
	).Scan(
		&existingActiveTripCount,
	); err != nil {
		t.Fatalf(
			"check existing active John trip: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Skip("John already has an active trip")
	}

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original John presence: %v",
			err,
		)
	}

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					latitude = $4,
					longitude = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore John presence: %v",
				restoreErr,
			)
		}
	}()

	futureHeartbeat :=
		time.Now().UTC().Add(5 * time.Minute)

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = $2,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
		futureHeartbeat,
	); err != nil {
		t.Fatalf(
			"prepare future-dated John presence: %v",
			err,
		)
	}

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'DispatchRide Future Heartbeat Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'DispatchRide future heartbeat regression test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create DispatchRide future-heartbeat ride: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide future-heartbeat trips: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide future-heartbeat ride: %v",
				cleanupErr,
			)
		}
	}()

	service := NewService(
		Dependencies{
			DB:     db,
			Config: cfg,
		},
	)

	trip, err := service.DispatchRide(
		ctx,
		rideRequestID,
	)

	if err != nil &&
		!errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {
		t.Fatalf(
			"unexpected DispatchRide error: %v",
			err,
		)
	}

	if trip != nil &&
		trip.DriverID == johnUserID {
		t.Fatalf(
			"future-dated John received dispatched trip %s",
			trip.ID,
		)
	}

	var johnTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE ride_request_id = $1
			  AND driver_id = $2
		`,
		rideRequestID,
		johnUserID,
	).Scan(
		&johnTripCount,
	); err != nil {
		t.Fatalf(
			"count future-dated John dispatched trips: %v",
			err,
		)
	}

	if johnTripCount != 0 {
		t.Fatalf(
			"expected zero trips for future-dated John, got %d",
			johnTripCount,
		)
	}

	var (
		persistedIsOnline  bool
		persistedStatus    string
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedIsOnline,
		&persistedStatus,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read John presence after future-heartbeat DispatchRide: %v",
			err,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected future-dated John to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected future-dated John to remain AVAILABLE, got %s",
			persistedStatus,
		)
	}

	if persistedHeartbeat == nil {
		t.Fatal(
			"expected future heartbeat to remain present",
		)
	}

	if !persistedHeartbeat.After(
		time.Now().UTC(),
	) {
		t.Fatalf(
			"expected John's heartbeat to remain future-dated, got %v",
			*persistedHeartbeat,
		)
	}

	var persistedRideStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&persistedRideStatus,
	); err != nil {
		t.Fatalf(
			"read ride after future-heartbeat DispatchRide: %v",
			err,
		)
	}

	if trip == nil &&
		persistedRideStatus != rideRequestStatusPending {
		t.Fatalf(
			"expected ride to remain %s when no driver was dispatched, got %s",
			rideRequestStatusPending,
			persistedRideStatus,
		)
	}

	if trip != nil &&
		persistedRideStatus != rideRequestStatusAccepted {
		t.Fatalf(
			"expected ride to be %s after another driver dispatch, got %s",
			rideRequestStatusAccepted,
			persistedRideStatus,
		)
	}
}

func TestDispatchRideRejectsDriverWithMissingHeartbeat(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"
		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	// ---------------------------------------------------------
	// Make sure John is not already committed to an active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE driver_id = $1
			  AND is_active = TRUE
			  AND deleted_at IS NULL
			  AND status NOT IN (
				'COMPLETED',
				'CANCELLED',
				'NO_DRIVER_AVAILABLE',
				'EXPIRED'
			  )
		`,
		johnUserID,
	).Scan(
		&existingActiveTripCount,
	); err != nil {
		t.Fatalf(
			"check existing active John trip: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Skip("John already has an active trip")
	}

	// ---------------------------------------------------------
	// Preserve John's presence state.
	// ---------------------------------------------------------

	var (
		originalIsOnline           bool
		originalAvailabilityStatus string
		originalLatitude           *float64
		originalLongitude          *float64
		originalHeartbeat          *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&originalIsOnline,
		&originalAvailabilityStatus,
		&originalLatitude,
		&originalLongitude,
		&originalHeartbeat,
	); err != nil {
		t.Fatalf(
			"load original John presence: %v",
			err,
		)
	}

	defer func() {
		if _, restoreErr := db.Exec(
			context.Background(),
			`
				UPDATE driver_presence
				SET
					is_online = $2,
					availability_status = $3,
					latitude = $4,
					longitude = $5,
					last_heartbeat_at = $6,
					updated_at = NOW()
				WHERE driver_id = $1
			`,
			johnUserID,
			originalIsOnline,
			originalAvailabilityStatus,
			originalLatitude,
			originalLongitude,
			originalHeartbeat,
		); restoreErr != nil {
			t.Logf(
				"restore John presence: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Make John otherwise eligible but remove his heartbeat.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE driver_presence
			SET
				is_online = TRUE,
				availability_status = 'AVAILABLE',
				latitude = 60.2055,
				longitude = 24.6559,
				last_heartbeat_at = NULL,
				updated_at = NOW()
			WHERE driver_id = $1
		`,
		johnUserID,
	); err != nil {
		t.Fatalf(
			"prepare missing-heartbeat John presence: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// Create disposable PENDING ride.
	// ---------------------------------------------------------

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

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
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'DispatchRide Missing Heartbeat Test',
				60.2055,
				24.6559,
				'Helsinki Central Station',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'DispatchRide missing heartbeat regression test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(10*time.Minute),
	)

	if err != nil {
		t.Fatalf(
			"create DispatchRide missing-heartbeat ride: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE ride_request_id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide missing-heartbeat trips: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup DispatchRide missing-heartbeat ride: %v",
				cleanupErr,
			)
		}
	}()

	service := NewService(
		Dependencies{
			DB:     db,
			Config: cfg,
		},
	)

	// ---------------------------------------------------------
	// Attempt direct dispatch.
	//
	// Another legitimate driver may still be selected.
	// John specifically must never be selected.
	// ---------------------------------------------------------

	trip, err := service.DispatchRide(
		ctx,
		rideRequestID,
	)

	if err != nil &&
		!errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {
		t.Fatalf(
			"unexpected DispatchRide error: %v",
			err,
		)
	}

	if trip != nil &&
		trip.DriverID == johnUserID {
		t.Fatalf(
			"missing-heartbeat John received dispatched trip %s",
			trip.ID,
		)
	}

	// ---------------------------------------------------------
	// Verify no persisted trip was assigned to John.
	// ---------------------------------------------------------

	var johnTripCount int

	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trips
			WHERE ride_request_id = $1
			  AND driver_id = $2
		`,
		rideRequestID,
		johnUserID,
	).Scan(
		&johnTripCount,
	); err != nil {
		t.Fatalf(
			"count missing-heartbeat John dispatched trips: %v",
			err,
		)
	}

	if johnTripCount != 0 {
		t.Fatalf(
			"expected zero trips for missing-heartbeat John, got %d",
			johnTripCount,
		)
	}

	// ---------------------------------------------------------
	// John's presence must remain unchanged by rejection.
	// ---------------------------------------------------------

	var (
		persistedIsOnline  bool
		persistedStatus    string
		persistedHeartbeat *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				last_heartbeat_at
			FROM driver_presence
			WHERE driver_id = $1
		`,
		johnUserID,
	).Scan(
		&persistedIsOnline,
		&persistedStatus,
		&persistedHeartbeat,
	); err != nil {
		t.Fatalf(
			"read John presence after missing-heartbeat DispatchRide: %v",
			err,
		)
	}

	if !persistedIsOnline {
		t.Fatal(
			"expected missing-heartbeat John to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected missing-heartbeat John to remain AVAILABLE, got %s",
			persistedStatus,
		)
	}

	if persistedHeartbeat != nil {
		t.Fatalf(
			"expected John's heartbeat to remain missing, got %v",
			*persistedHeartbeat,
		)
	}

	// ---------------------------------------------------------
	// Ride state depends on whether another valid driver won.
	// ---------------------------------------------------------

	var persistedRideStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(
		&persistedRideStatus,
	); err != nil {
		t.Fatalf(
			"read ride after missing-heartbeat DispatchRide: %v",
			err,
		)
	}

	if trip == nil &&
		persistedRideStatus != rideRequestStatusPending {
		t.Fatalf(
			"expected ride to remain %s when no driver was dispatched, got %s",
			rideRequestStatusPending,
			persistedRideStatus,
		)
	}

	if trip != nil &&
		persistedRideStatus != rideRequestStatusAccepted {
		t.Fatalf(
			"expected ride to be %s after another driver dispatch, got %s",
			rideRequestStatusAccepted,
			persistedRideStatus,
		)
	}
}
