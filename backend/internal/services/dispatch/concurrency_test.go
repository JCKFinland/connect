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
