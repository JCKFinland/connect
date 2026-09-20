package trip

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
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestRecordTripLocationPersistsAuthenticatedDriverEvidence(
	t *testing.T,
) {
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
	// 3. Create an isolated driver fixture.
	// ---------------------------------------------------------

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create isolated driver fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupDriverFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup isolated driver fixture: %v",
				err,
			)
		}
	}()

	driverUserID := driverFixture.UserID

	// ---------------------------------------------------------
	// 4. Ensure the isolated user has DRIVER authorization.
	// ---------------------------------------------------------

	var driverRoleID string

	err = db.QueryRow(
		ctx,
		`
		SELECT id
		FROM roles
		WHERE name = 'DRIVER'
		LIMIT 1
	`,
	).Scan(
		&driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"resolve DRIVER role: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO user_roles
		(
			user_id,
			role_id
		)
		VALUES
		(
			$1,
			$2
		)
		ON CONFLICT DO NOTHING
	`,
		driverUserID,
		driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"grant DRIVER role to isolated user: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 5. Use the isolated driver's assignment hierarchy.
	// ---------------------------------------------------------

	vehicleID := driverFixture.VehicleID
	companyID := driverFixture.CompanyID
	branchID := driverFixture.BranchID
	fleetID := driverFixture.FleetID

	// ---------------------------------------------------------
	// 6. Assert the isolated driver starts without an active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	err = db.QueryRow(
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
		driverUserID,
	).Scan(
		&existingActiveTripCount,
	)
	if err != nil {
		t.Fatalf(
			"check isolated driver's active trips: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Fatalf(
			"isolated driver unexpectedly has %d active trip(s)",
			existingActiveTripCount,
		)
	}
	// ---------------------------------------------------------
	// 8. Create disposable ride request and IN_PROGRESS trip.
	// ---------------------------------------------------------

	now := time.Now().UTC()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				'GPS Ingestion Test Pickup',
				60.1708,
				24.9375,
				'GPS Ingestion Test Destination',
				60.1718,
				24.9475,
				'STANDARD',
				1,
				'ACCEPTED',
				'Authenticated GPS ingestion integration test',
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
			"create GPS ingestion ride request: %v",
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
				"cleanup GPS ingestion ride request: %v",
				cleanupErr,
			)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips
			(
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				assigned_at,
				started_at,
				is_active,
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
				$8,
				'IN_PROGRESS',
				$9,
				$9,
				TRUE,
				$9,
				$9
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create GPS ingestion trip: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup GPS ingestion trip: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Construct production service dependencies.
	// ---------------------------------------------------------

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	service := NewService(
		Dependencies{
			DB: db,

			Trips: postgresrepo.NewTripRepository(db),

			RideRequests: postgresrepo.NewRideRequestRepository(db),

			Presence: postgresrepo.NewDriverPresenceRepository(db),

			TripEvents: postgresrepo.NewTripEventRepository(db),

			UserRoles: userRoleRepo,

			TripLocations: postgresrepo.NewTripLocationRepository(db),
		},
	)

	// ---------------------------------------------------------
	// 10. Submit GPS evidence as the authenticated isolated driver.
	// ---------------------------------------------------------

	accuracy := 5.0
	speed := 22.5
	altitude := 18.0
	heading := 90

	recordedAt := time.Now().UTC().
		Truncate(time.Microsecond)

	location, err := service.RecordTripLocation(
		ctx,
		tripID,
		driverUserID,
		RecordLocationRequest{
			Latitude:       60.1708,
			Longitude:      24.9375,
			Altitude:       &altitude,
			SpeedKMH:       &speed,
			Heading:        &heading,
			AccuracyMeters: &accuracy,
			RecordedAt:     recordedAt,
		},
	)
	if err != nil {
		t.Fatalf(
			"record trip location: %v",
			err,
		)
	}

	if location == nil {
		t.Fatal(
			"expected recorded trip location",
		)
	}

	// ---------------------------------------------------------
	// 11. Verify returned identity is server-derived.
	// ---------------------------------------------------------

	if location.ID == "" {
		t.Fatal(
			"expected generated trip location ID",
		)
	}

	if location.TripID != tripID {
		t.Fatalf(
			"expected returned trip ID %s, got %s",
			tripID,
			location.TripID,
		)
	}

	if location.DriverID != driverUserID {
		t.Fatalf(
			"expected returned driver ID %s, got %s",
			driverUserID,
			location.DriverID,
		)
	}

	// ---------------------------------------------------------
	// 12. Replaying the same physical GPS observation must be
	//     idempotent.
	// ---------------------------------------------------------

	duplicateLocation, err := service.RecordTripLocation(
		ctx,
		tripID,
		driverUserID,
		RecordLocationRequest{
			Latitude:       60.1708,
			Longitude:      24.9375,
			Altitude:       &altitude,
			SpeedKMH:       &speed,
			Heading:        &heading,
			AccuracyMeters: &accuracy,
			RecordedAt:     recordedAt,
		},
	)
	if err != nil {
		t.Fatalf(
			"replay duplicate trip location: %v",
			err,
		)
	}

	if duplicateLocation == nil {
		t.Fatal(
			"expected persisted location for duplicate observation",
		)
	}

	if duplicateLocation.ID != location.ID {
		t.Fatalf(
			"expected duplicate observation to return persisted location %s, got %s",
			location.ID,
			duplicateLocation.ID,
		)
	}

	var duplicateObservationCount int

	err = db.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM trip_locations
		WHERE trip_id = $1
		  AND driver_id = $2
		  AND recorded_at = $3
	`,
		tripID,
		driverUserID,
		recordedAt,
	).Scan(&duplicateObservationCount)
	if err != nil {
		t.Fatalf(
			"count duplicate trip location observations: %v",
			err,
		)
	}

	if duplicateObservationCount != 1 {
		t.Fatalf(
			"expected exactly 1 persisted observation after duplicate replay, got %d",
			duplicateObservationCount,
		)
	}

	// ---------------------------------------------------------
	// 13. Verify immutable persisted GPS evidence.
	// ---------------------------------------------------------

	var (
		persistedID             string
		persistedTripID         string
		persistedDriverID       string
		persistedLatitude       float64
		persistedLongitude      float64
		persistedAltitude       *float64
		persistedSpeedKMH       *float64
		persistedHeading        *int16
		persistedAccuracyMeters *float64
		persistedRecordedAt     time.Time
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				id,
				trip_id,
				driver_id,
				latitude,
				longitude,
				altitude,
				speed_kmh,
				heading,
				accuracy_meters,
				recorded_at
			FROM trip_locations
			WHERE id = $1
		`,
		location.ID,
	).Scan(
		&persistedID,
		&persistedTripID,
		&persistedDriverID,
		&persistedLatitude,
		&persistedLongitude,
		&persistedAltitude,
		&persistedSpeedKMH,
		&persistedHeading,
		&persistedAccuracyMeters,
		&persistedRecordedAt,
	)
	if err != nil {
		t.Fatalf(
			"read persisted trip location: %v",
			err,
		)
	}

	if persistedID != location.ID {
		t.Fatalf(
			"expected persisted location ID %s, got %s",
			location.ID,
			persistedID,
		)
	}

	if persistedTripID != tripID {
		t.Fatalf(
			"expected persisted trip ID %s, got %s",
			tripID,
			persistedTripID,
		)
	}

	if persistedDriverID != driverUserID {
		t.Fatalf(
			"expected persisted authenticated driver ID %s, got %s",
			driverUserID,
			persistedDriverID,
		)
	}

	if persistedLatitude != 60.1708 {
		t.Fatalf(
			"expected latitude 60.1708, got %f",
			persistedLatitude,
		)
	}

	if persistedLongitude != 24.9375 {
		t.Fatalf(
			"expected longitude 24.9375, got %f",
			persistedLongitude,
		)
	}

	if persistedAltitude == nil ||
		*persistedAltitude != altitude {
		t.Fatalf(
			"expected altitude %.2f, got %v",
			altitude,
			persistedAltitude,
		)
	}

	if persistedSpeedKMH == nil ||
		*persistedSpeedKMH != speed {
		t.Fatalf(
			"expected speed %.2f, got %v",
			speed,
			persistedSpeedKMH,
		)
	}

	if persistedHeading == nil ||
		*persistedHeading != int16(heading) {
		t.Fatalf(
			"expected heading %d, got %v",
			heading,
			persistedHeading,
		)
	}

	if persistedAccuracyMeters == nil ||
		*persistedAccuracyMeters != accuracy {
		t.Fatalf(
			"expected accuracy %.2f, got %v",
			accuracy,
			persistedAccuracyMeters,
		)
	}

	if !persistedRecordedAt.Equal(recordedAt) {
		t.Fatalf(
			"expected recorded_at %s, got %s",
			recordedAt,
			persistedRecordedAt,
		)
	}

	// ---------------------------------------------------------
	// 14. Exactly one evidence row must exist for this test trip.
	// ---------------------------------------------------------

	var locationCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_locations
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&locationCount,
	)
	if err != nil {
		t.Fatalf(
			"count persisted trip locations: %v",
			err,
		)
	}

	if locationCount != 1 {
		t.Fatalf(
			"expected exactly 1 trip location, got %d",
			locationCount,
		)
	}

	// ---------------------------------------------------------
	// 15. A non-DRIVER actor must not record GPS evidence.
	// ---------------------------------------------------------

	customerRoles, err := userRoleRepo.GetUserRoles(
		ctx,
		customerID,
	)
	if err != nil {
		t.Fatalf(
			"load customer roles: %v",
			err,
		)
	}

	customerHasDriverRole := false

	for _, role := range customerRoles {
		if role == "DRIVER" {
			customerHasDriverRole = true
			break
		}
	}

	if customerHasDriverRole {
		t.Fatalf(
			"customer fixture unexpectedly has DRIVER role",
		)
	}

	_, err = service.RecordTripLocation(
		ctx,
		tripID,
		customerID,
		RecordLocationRequest{
			Latitude:       60.1709,
			Longitude:      24.9376,
			AccuracyMeters: &accuracy,
			RecordedAt:     time.Now().UTC(),
		},
	)

	if !errors.Is(
		err,
		ErrTripLocationAccessDenied,
	) {
		t.Fatalf(
			"expected ErrTripLocationAccessDenied for non-driver actor, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 16. A DRIVER must not record evidence for another user's
	//     trip.
	//
	// This disposable trip is temporarily assigned to the
	// customer user solely to exercise the ownership boundary.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET driver_id = $1
			WHERE id = $2
		`,
		customerID,
		tripID,
	)
	if err != nil {
		t.Fatalf(
			"temporarily assign trip to different user: %v",
			err,
		)
	}

	_, err = service.RecordTripLocation(
		ctx,
		tripID,
		driverUserID,
		RecordLocationRequest{
			Latitude:       60.1710,
			Longitude:      24.9377,
			AccuracyMeters: &accuracy,
			RecordedAt:     time.Now().UTC(),
		},
	)

	if !errors.Is(
		err,
		ErrTripLocationAccessDenied,
	) {
		t.Fatalf(
			"expected ErrTripLocationAccessDenied for another user's trip, got %v",
			err,
		)
	}

	// Restore authoritative driver ownership for the lifecycle test.
	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET driver_id = $1
			WHERE id = $2
		`,
		driverUserID,
		tripID,
	)
	if err != nil {
		t.Fatalf(
			"restore isolated driver as trip driver: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 17. GPS evidence must only be accepted while the trip is
	//     IN_PROGRESS.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			UPDATE trips
			SET status = $1
			WHERE id = $2
		`,
		StatusAssigned,
		tripID,
	)
	if err != nil {
		t.Fatalf(
			"set trip to ASSIGNED: %v",
			err,
		)
	}

	_, err = service.RecordTripLocation(
		ctx,
		tripID,
		driverUserID,
		RecordLocationRequest{
			Latitude:       60.1711,
			Longitude:      24.9378,
			AccuracyMeters: &accuracy,
			RecordedAt:     time.Now().UTC(),
		},
	)

	if !errors.Is(
		err,
		ErrTripNotInProgress,
	) {
		t.Fatalf(
			"expected ErrTripNotInProgress, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 18. Rejected attempts must not create additional evidence.
	// ---------------------------------------------------------

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_locations
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&locationCount,
	)
	if err != nil {
		t.Fatalf(
			"count trip locations after rejected attempts: %v",
			err,
		)
	}

	if locationCount != 1 {
		t.Fatalf(
			"expected rejected attempts to leave exactly 1 location, got %d",
			locationCount,
		)
	}
}

func TestRecordTripLocationSerializesAgainstTripCompletion(
	t *testing.T,
) {
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
	// 3. Create an isolated driver fixture.
	// ---------------------------------------------------------

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create isolated driver fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupDriverFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup isolated driver fixture: %v",
				err,
			)
		}
	}()

	driverUserID := driverFixture.UserID

	// ---------------------------------------------------------
	// 4. Ensure the isolated user has DRIVER authorization.
	// ---------------------------------------------------------

	var driverRoleID string

	err = db.QueryRow(
		ctx,
		`
			SELECT id
			FROM roles
			WHERE name = 'DRIVER'
			LIMIT 1
		`,
	).Scan(
		&driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"resolve DRIVER role: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO user_roles
			(
				user_id,
				role_id
			)
			VALUES
			(
				$1,
				$2
			)
			ON CONFLICT DO NOTHING
		`,
		driverUserID,
		driverRoleID,
	)
	if err != nil {
		t.Fatalf(
			"grant DRIVER role to isolated user: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 5. Use the isolated driver's assignment hierarchy.
	// ---------------------------------------------------------

	vehicleID := driverFixture.VehicleID
	companyID := driverFixture.CompanyID
	branchID := driverFixture.BranchID
	fleetID := driverFixture.FleetID

	// ---------------------------------------------------------
	// 6. Assert the isolated driver starts without an active trip.
	// ---------------------------------------------------------

	var existingActiveTripCount int

	err = db.QueryRow(
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
		driverUserID,
	).Scan(
		&existingActiveTripCount,
	)
	if err != nil {
		t.Fatalf(
			"check isolated driver's active trips: %v",
			err,
		)
	}

	if existingActiveTripCount != 0 {
		t.Fatalf(
			"isolated driver unexpectedly has %d active trip(s)",
			existingActiveTripCount,
		)
	}

	// ---------------------------------------------------------
	// 8. Create disposable ride request and IN_PROGRESS trip.
	// ---------------------------------------------------------

	now := time.Now().UTC()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				'GPS Serialization Test Pickup',
				60.1708,
				24.9375,
				'GPS Serialization Test Destination',
				60.1718,
				24.9475,
				'STANDARD',
				1,
				'ACCEPTED',
				'GPS/completion serialization regression test',
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
			"create serialization ride request: %v",
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
				"cleanup serialization ride request: %v",
				cleanupErr,
			)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips
			(
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				assigned_at,
				started_at,
				is_active,
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
				$8,
				'IN_PROGRESS',
				$9,
				$9,
				TRUE,
				$9,
				$9
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create serialization trip: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup serialization trip: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 9. Construct production service.
	// ---------------------------------------------------------

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	service := NewService(
		Dependencies{
			DB: db,

			Trips: postgresrepo.NewTripRepository(db),

			RideRequests: postgresrepo.NewRideRequestRepository(db),

			Presence: postgresrepo.NewDriverPresenceRepository(db),

			TripEvents: postgresrepo.NewTripEventRepository(db),

			UserRoles: userRoleRepo,

			TripLocations: postgresrepo.NewTripLocationRepository(db),
		},
	)

	// ---------------------------------------------------------
	// 10. Begin completion-side transaction and lock trip row.
	// ---------------------------------------------------------

	completionTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf(
			"begin completion transaction: %v",
			err,
		)
	}

	completionCommitted := false

	defer func() {
		if !completionCommitted {
			_ = completionTx.Rollback(
				context.Background(),
			)
		}
	}()

	completionTrips :=
		postgresrepo.NewTripRepositoryWithDB(
			completionTx,
		)

	lockedTrip, err :=
		completionTrips.GetByIDForUpdate(
			ctx,
			tripID,
		)
	if err != nil {
		t.Fatalf(
			"lock trip for simulated completion: %v",
			err,
		)
	}

	if lockedTrip.Status != StatusInProgress {
		t.Fatalf(
			"expected locked trip status %s, got %s",
			StatusInProgress,
			lockedTrip.Status,
		)
	}

	// ---------------------------------------------------------
	// 11. Start GPS ingestion while completion owns row lock.
	// ---------------------------------------------------------

	accuracy := 5.0

	recordedAt := time.Now().UTC().
		Truncate(time.Microsecond)

	type locationResult struct {
		location *models.TripLocation
		err      error
	}

	resultCh := make(
		chan locationResult,
		1,
	)

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		location, recordErr :=
			service.RecordTripLocation(
				context.Background(),
				tripID,
				driverUserID,
				RecordLocationRequest{
					Latitude:       60.1708,
					Longitude:      24.9375,
					AccuracyMeters: &accuracy,
					RecordedAt:     recordedAt,
				},
			)

		resultCh <- locationResult{
			location: location,
			err:      recordErr,
		}
	}()

	// The request must not be able to finish while the completion
	// transaction owns the authoritative trip row lock.
	select {
	case result := <-resultCh:
		t.Fatalf(
			"location recording completed while completion held trip lock: location=%v err=%v",
			result.location,
			result.err,
		)

	case <-time.After(250 * time.Millisecond):
		// Expected: RecordTripLocation is blocked on FOR UPDATE.
	}

	// ---------------------------------------------------------
	// 12. Complete trip while still owning the same row lock.
	// ---------------------------------------------------------

	if err := completionTrips.UpdateStatus(
		ctx,
		tripID,
		StatusCompleted,
	); err != nil {
		t.Fatalf(
			"mark locked trip completed: %v",
			err,
		)
	}

	if err := completionTx.Commit(ctx); err != nil {
		t.Fatalf(
			"commit simulated completion: %v",
			err,
		)
	}

	completionCommitted = true

	// ---------------------------------------------------------
	// 13. GPS request must wake, observe COMPLETED, and reject.
	// ---------------------------------------------------------

	var result locationResult

	select {
	case result = <-resultCh:

	case <-time.After(5 * time.Second):
		t.Fatal(
			"timed out waiting for blocked location request",
		)
	}

	wg.Wait()

	if !errors.Is(
		result.err,
		ErrTripNotInProgress,
	) {
		t.Fatalf(
			"expected ErrTripNotInProgress after completion won lock, got %v",
			result.err,
		)
	}

	if result.location != nil {
		t.Fatalf(
			"expected no location after completion, got %+v",
			result.location,
		)
	}

	// ---------------------------------------------------------
	// 14. Verify no post-completion GPS evidence was persisted.
	// ---------------------------------------------------------

	var locationCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM trip_locations
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&locationCount,
	)
	if err != nil {
		t.Fatalf(
			"count serialization trip locations: %v",
			err,
		)
	}

	if locationCount != 0 {
		t.Fatalf(
			"expected zero post-completion location rows, got %d",
			locationCount,
		)
	}

	// ---------------------------------------------------------
	// 15. Verify authoritative lifecycle result.
	// ---------------------------------------------------------

	var (
		persistedStatus      string
		persistedIsActive    bool
		persistedCompletedAt *time.Time
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				status,
				is_active,
				completed_at
			FROM trips
			WHERE id = $1
		`,
		tripID,
	).Scan(
		&persistedStatus,
		&persistedIsActive,
		&persistedCompletedAt,
	)
	if err != nil {
		t.Fatalf(
			"read completed serialization trip: %v",
			err,
		)
	}

	if persistedStatus != StatusCompleted {
		t.Fatalf(
			"expected persisted status %s, got %s",
			StatusCompleted,
			persistedStatus,
		)
	}

	if persistedIsActive {
		t.Fatal(
			"expected completed trip to be inactive",
		)
	}

	if persistedCompletedAt == nil {
		t.Fatal(
			"expected completed_at after simulated completion",
		)
	}
}
