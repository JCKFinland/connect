package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTripMeterMeasurementRepositoryCreateAndGetByTripID(
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
	// 3. Serialize John's shared integration fixture.
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

	// ---------------------------------------------------------
	// 4. Controlled shared fixture IDs.
	//
	// trips.driver_id references users.id.
	// ---------------------------------------------------------

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

		johnUserID = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	// ---------------------------------------------------------
	// 5. Resolve John's current active assignment.
	//
	// Do not hardcode vehicle/company/branch/fleet because the
	// shared integration fixture may legitimately evolve.
	// ---------------------------------------------------------

	var (
		vehicleID string
		companyID string
		branchID  string
		fleetID   string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				vehicle_id,
				company_id,
				branch_id,
				fleet_id
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		`,
		johnUserID,
	).Scan(
		&vehicleID,
		&companyID,
		&branchID,
		&fleetID,
	)
	if err != nil {
		t.Fatalf(
			"resolve John's active assignment: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 6. Create disposable service category and pricing profile.
	// ---------------------------------------------------------

	now := time.Now().UTC()

	serviceCategoryID := uuid.NewString()
	pricingProfileID := uuid.NewString()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

	pricingVersion :=
		"meter-" + uuid.NewString()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO service_categories (
				id,
				code,
				name,
				description,
				is_active,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				'Trip Meter Repository Test',
				'Trip meter measurement repository integration test',
				TRUE,
				$3,
				$3
			)
		`,
		serviceCategoryID,
		"METER_"+uuid.NewString()[:8],
		now,
	)
	if err != nil {
		t.Fatalf(
			"create service category: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO fare_pricing_profiles (
				id,
				company_id,
				branch_id,
				service_category_id,
				version,
				currency,
				base_fare,
				distance_rate_per_km,
				time_rate_per_minute,
				waiting_rate_per_minute,
				booking_fee,
				surge_multiplier,
				effective_from,
				effective_to,
				is_active,
				created_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				'EUR',
				4.90,
				1.50,
				0.25,
				0.50,
				2.00,
				1.00,
				$6,
				NULL,
				TRUE,
				$7
			)
		`,
		pricingProfileID,
		companyID,
		branchID,
		serviceCategoryID,
		pricingVersion,
		now.Add(-time.Hour),
		now,
	)
	if err != nil {
		t.Fatalf(
			"create pricing profile: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 7. Create disposable ride request.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
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
				service_category_id,
				passenger_count,
				status,
				notes,
				requested_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				'Trip Meter Repository Test Pickup',
				60.1700,
				24.9300,
				'Trip Meter Repository Test Destination',
				60.1710,
				24.9310,
				'STANDARD',
				$3,
				1,
				'ACCEPTED',
				'Trip meter repository integration test',
				$4,
				$5,
				$4,
				$4
			)
		`,
		rideRequestID,
		customerID,
		serviceCategoryID,
		now,
		now.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatalf(
			"create ride request: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. Create disposable trip.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips (
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				service_category_id,
				pricing_profile_id,
				status,
				assigned_at,
				started_at,
				is_active,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9,
				$10,
				'IN_PROGRESS',
				$11,
				$11,
				TRUE,
				$11,
				$11
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		johnUserID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		serviceCategoryID,
		pricingProfileID,
		now,
	)
	if err != nil {
		t.Fatalf(
			"create trip: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 9. Cleanup disposable fixture.
	//
	// Deleting the trip cascades trip_meter_measurements.
	// ---------------------------------------------------------

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup trip: %v",
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

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM fare_pricing_profiles
				WHERE id = $1
			`,
			pricingProfileID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup pricing profile: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM service_categories
				WHERE id = $1
			`,
			serviceCategoryID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup service category: %v",
				cleanupErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// 10. Create repository.
	// ---------------------------------------------------------

	repo :=
		NewTripMeterMeasurementRepository(db)

	// ---------------------------------------------------------
	// 11. Persist authoritative measurement snapshot.
	// ---------------------------------------------------------

	measuredAt :=
		now.Add(5 * time.Minute)

	measurement :=
		&models.TripMeterMeasurement{
			TripID: tripID,

			MeasurementSource: "GPS",
			AlgorithmVersion:  "gps-v1",

			DistanceMeters:         1234,
			DurationSeconds:        567,
			WaitingDurationSeconds: 89,

			AcceptedSamples:  10,
			RejectedSamples:  2,
			RejectedSegments: 1,

			MeasuredAt: measuredAt,
		}

	err = repo.Create(
		ctx,
		measurement,
	)
	if err != nil {
		t.Fatalf(
			"create trip meter measurement: %v",
			err,
		)
	}

	if measurement.ID == "" {
		t.Fatal(
			"expected generated measurement ID",
		)
	}

	if measurement.CreatedAt.IsZero() {
		t.Fatal(
			"expected measurement created_at",
		)
	}

	// ---------------------------------------------------------
	// 12. Read snapshot back by trip ID.
	// ---------------------------------------------------------

	persisted, err :=
		repo.GetByTripID(
			ctx,
			tripID,
		)
	if err != nil {
		t.Fatalf(
			"get trip meter measurement: %v",
			err,
		)
	}

	if persisted == nil {
		t.Fatal(
			"expected persisted trip meter measurement",
		)
	}

	// ---------------------------------------------------------
	// 13. Verify authoritative fields round-trip.
	// ---------------------------------------------------------

	if persisted.ID != measurement.ID {
		t.Fatalf(
			"expected measurement ID %s, got %s",
			measurement.ID,
			persisted.ID,
		)
	}

	if persisted.TripID != tripID {
		t.Fatalf(
			"expected trip ID %s, got %s",
			tripID,
			persisted.TripID,
		)
	}

	if persisted.MeasurementSource != "GPS" {
		t.Fatalf(
			"expected measurement source GPS, got %s",
			persisted.MeasurementSource,
		)
	}

	if persisted.AlgorithmVersion != "gps-v1" {
		t.Fatalf(
			"expected algorithm version gps-v1, got %s",
			persisted.AlgorithmVersion,
		)
	}

	if persisted.DistanceMeters != 1234 {
		t.Fatalf(
			"expected distance 1234, got %d",
			persisted.DistanceMeters,
		)
	}

	if persisted.DurationSeconds != 567 {
		t.Fatalf(
			"expected duration 567, got %d",
			persisted.DurationSeconds,
		)
	}

	if persisted.WaitingDurationSeconds != 89 {
		t.Fatalf(
			"expected waiting duration 89, got %d",
			persisted.WaitingDurationSeconds,
		)
	}

	if persisted.AcceptedSamples != 10 {
		t.Fatalf(
			"expected accepted samples 10, got %d",
			persisted.AcceptedSamples,
		)
	}

	if persisted.RejectedSamples != 2 {
		t.Fatalf(
			"expected rejected samples 2, got %d",
			persisted.RejectedSamples,
		)
	}

	if persisted.RejectedSegments != 1 {
		t.Fatalf(
			"expected rejected segments 1, got %d",
			persisted.RejectedSegments,
		)
	}

	expectedMeasuredAt :=
		measuredAt.Truncate(time.Microsecond)

	if !persisted.MeasuredAt.Equal(expectedMeasuredAt) {
		t.Fatalf(
			"expected measured_at %s, got %s",
			expectedMeasuredAt,
			persisted.MeasuredAt,
		)
	}

	if persisted.CreatedAt.IsZero() {
		t.Fatal(
			"expected persisted created_at",
		)
	}

	// ---------------------------------------------------------
	// 14. A trip may have only one authoritative measurement.
	// ---------------------------------------------------------

	duplicate :=
		&models.TripMeterMeasurement{
			TripID: tripID,

			MeasurementSource: "GPS",
			AlgorithmVersion:  "gps-v1",

			DistanceMeters:         9999,
			DurationSeconds:        9999,
			WaitingDurationSeconds: 9999,

			AcceptedSamples: 1,

			MeasuredAt: now.Add(6 * time.Minute),
		}

	err = repo.Create(
		ctx,
		duplicate,
	)
	if err == nil {
		t.Fatal(
			"expected duplicate trip meter measurement to fail",
		)
	}

	// ---------------------------------------------------------
	// 15. Original immutable snapshot must remain unchanged.
	// ---------------------------------------------------------

	original, err :=
		repo.GetByTripID(
			ctx,
			tripID,
		)
	if err != nil {
		t.Fatalf(
			"get original measurement after duplicate failure: %v",
			err,
		)
	}

	if original.ID != measurement.ID {
		t.Fatalf(
			"expected original measurement ID %s, got %s",
			measurement.ID,
			original.ID,
		)
	}

	if original.DistanceMeters != 1234 {
		t.Fatalf(
			"expected original distance 1234, got %d",
			original.DistanceMeters,
		)
	}

	if original.DurationSeconds != 567 {
		t.Fatalf(
			"expected original duration 567, got %d",
			original.DurationSeconds,
		)
	}

	if original.WaitingDurationSeconds != 89 {
		t.Fatalf(
			"expected original waiting duration 89, got %d",
			original.WaitingDurationSeconds,
		)
	}
}
