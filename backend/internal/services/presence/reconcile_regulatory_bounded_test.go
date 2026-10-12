package presence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestReconcileRegulatoryBoundedOfflinesExpiredDriverWithFreshHeartbeat(
	t *testing.T,
) {
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

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		)

		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		)

		if err := cleanup(cleanupCtx); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)
	expired := now.AddDate(0, 0, -1)

	_, err = db.Exec(ctx, `
		UPDATE drivers
		SET driving_license_expiry = $2,
		    is_active = TRUE,
		    is_verified = TRUE,
		    status = 'ACTIVE'
		WHERE id = $1
	`, fixture.DriverID, expired)
	if err != nil {
		t.Fatalf("configure expired driver: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id,
			company_id,
			branch_id,
			vehicle_id,
			assignment_id,
			is_online,
			availability_status,
			last_heartbeat_at
		)
		VALUES ($1, $2, $3, $4, $5, TRUE, 'AVAILABLE', $6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("insert fresh online presence: %v", err)
	}

	service := &Service{
		db:  db,
		cfg: cfg,
		complianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
			return driver_compliance.NewService(
				driver_compliance.Dependencies{
					Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
					Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
				},
			)
		},
	}

	transitioned, err := service.ReconcileRegulatoryBounded(
		ctx,
		now,
		10000,
	)
	if err != nil {
		t.Fatalf("reconcile expired driver: %v", err)
	}

	var online bool
	var status string

	err = db.QueryRow(ctx, `
		SELECT is_online, availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(&online, &status)
	if err != nil {
		t.Fatalf("read reconciled presence: %v", err)
	}

	if online || status != "OFFLINE" {
		t.Fatalf(
			"expected expired driver OFFLINE, got online=%v status=%s",
			online,
			status,
		)
	}

	if transitioned < 1 {
		t.Fatalf(
			"expected at least one offline transition, got %d",
			transitioned,
		)
	}
}

func TestRegulatoryCandidateDiscoveryUsesUTCCalendarDate(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		)

		if err := cleanup(cleanupCtx); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()

	// Use a fixed UTC timestamp near the end of the day.
	now := time.Date(
		2026, time.October, 12,
		23, 59, 59, 0,
		time.UTC,
	)

	_, err = db.Exec(ctx, `
		UPDATE drivers
		SET is_active = TRUE,
		    is_verified = TRUE,
		    status = 'ACTIVE',
		    driving_license_expiry = $2
		WHERE id = $1
	`, fixture.DriverID, now)
	if err != nil {
		t.Fatalf("configure driver: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			vehicle_id, assignment_id,
			is_online, availability_status,
			last_heartbeat_at
		)
		VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("insert presence: %v", err)
	}

	// Supply both verified documents so their absence cannot
	// mask an incorrect driving-licence date comparison.
	for _, documentType := range []string{
		"DRIVING_LICENSE",
		"TAXI_DRIVER_LICENSE",
	} {
		_, err = db.Exec(ctx, `
			INSERT INTO driver_documents (
				driver_id,
				document_type,
				file_name,
				storage_key,
				content_type,
				file_size_bytes,
				status,
				expires_at,
				verified_at,
				verified_by_user_id
			)
			VALUES (
				$1, $2, 'boundary-test.pdf',
				$3, 'application/pdf',
				4096, 'VERIFIED', $4, $5, $6
			)
		`,
			fixture.DriverID,
			documentType,
			"drivers/"+fixture.DriverID+"/"+documentType+"/boundary-test.pdf",
			now.AddDate(0, 0, 1),
			now,
			fixture.UserID,
		)
		if err != nil {
			t.Fatalf("insert verified %s: %v", documentType, err)
		}
	}

	repo := postgresrepo.NewDriverPresenceRepositoryWithDB(db)

	lister, ok := repo.(interface {
		ListRegulatoryCandidates(
			context.Context,
			time.Time,
			string,
			int,
		) ([]postgresrepo.RegulatoryDriverCandidate, error)
	})
	if !ok {
		t.Fatal("repository does not support regulatory discovery")
	}

	containsFixture := func(candidates []postgresrepo.RegulatoryDriverCandidate) bool {
		for _, candidate := range candidates {
			if candidate.DriverID == fixture.DriverID {
				return true
			}
		}
		return false
	}

	todayCandidates, err := lister.ListRegulatoryCandidates(
		ctx, now, "", 10000,
	)
	if err != nil {
		t.Fatalf("discover candidates expiring today: %v", err)
	}

	if containsFixture(todayCandidates) {
		t.Fatal("driver expiring today was incorrectly marked expired")
	}

	_, err = db.Exec(ctx, `
		UPDATE drivers
		SET driving_license_expiry = $2
		WHERE id = $1
	`, fixture.DriverID, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("expire driving licence yesterday: %v", err)
	}

	expiredCandidates, err := lister.ListRegulatoryCandidates(
		ctx, now, "", 10000,
	)
	if err != nil {
		t.Fatalf("discover expired candidates: %v", err)
	}

	if !containsFixture(expiredCandidates) {
		t.Fatal("driver expired yesterday was not discovered")
	}
}

func TestReconcileRegulatoryBoundedSkipsLockedDriver(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		)

		if err := cleanup(cleanupCtx); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			vehicle_id, assignment_id,
			is_online, availability_status,
			last_heartbeat_at
		)
		VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("insert online presence: %v", err)
	}

	service := &Service{
		db:  db,
		cfg: cfg,
		complianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
			return driver_compliance.NewService(
				driver_compliance.Dependencies{
					Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
					Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
				},
			)
		},
	}

	lockTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin locking transaction: %v", err)
	}

	var lockedID string

	err = lockTx.QueryRow(ctx, `
		SELECT id FROM drivers
		WHERE id = $1
		FOR UPDATE
	`, fixture.DriverID).Scan(&lockedID)
	if err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("lock driver aggregate: %v", err)
	}

	lockedCtx, cancel := context.WithTimeout(
		ctx,
		3*time.Second,
	)

	transitioned, err := service.ReconcileRegulatoryBounded(
		lockedCtx,
		now,
		10000,
	)
	cancel()

	if err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("reconciliation blocked by driver lock: %v", err)
	}

	if transitioned != 0 {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf(
			"expected no transition while locked, got %d",
			transitioned,
		)
	}

	var online bool

	err = db.QueryRow(ctx, `
		SELECT is_online
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(&online)
	if err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("read locked presence: %v", err)
	}

	if !online {
		_ = lockTx.Rollback(context.Background())
		t.Fatal("locked driver was incorrectly taken offline")
	}

	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release driver lock: %v", err)
	}

	// First page was consumed while the aggregate was locked.
	// A subsequent cycle must eventually revisit that driver.
	transitioned, err = service.ReconcileRegulatoryBounded(
		ctx,
		now,
		10000,
	)
	if err != nil {
		t.Fatalf("reconcile unlocked driver: %v", err)
	}

	var status string

	err = db.QueryRow(ctx, `
		SELECT is_online, availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(&online, &status)
	if err != nil {
		t.Fatalf("read reconciled presence: %v", err)
	}

	if online || status != "OFFLINE" {
		t.Fatalf(
			"expected unlocked driver OFFLINE, got online=%v status=%s",
			online,
			status,
		)
	}

	if transitioned < 1 {
		t.Fatalf(
			"expected offline transition after unlock, got %d",
			transitioned,
		)
	}
}

func TestReconcileRegulatoryBoundedPreservesBusyDriver(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		)

		if err := cleanup(cleanupCtx); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = db.Exec(ctx, `
		UPDATE drivers
		SET driving_license_expiry = $2
		WHERE id = $1
	`, fixture.DriverID, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("expire driver licence: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id,
			company_id,
			branch_id,
			vehicle_id,
			assignment_id,
			is_online,
			availability_status,
			last_heartbeat_at
		)
		VALUES ($1,$2,$3,$4,$5,TRUE,'BUSY',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("insert BUSY presence: %v", err)
	}

	service := &Service{
		db:  db,
		cfg: cfg,
		complianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
			return driver_compliance.NewService(
				driver_compliance.Dependencies{
					Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
					Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
				},
			)
		},
	}

	transitioned, err := service.ReconcileRegulatoryBounded(
		ctx,
		now,
		10000,
	)
	if err != nil {
		t.Fatalf("reconcile BUSY driver: %v", err)
	}

	var online bool
	var status string

	err = db.QueryRow(ctx, `
		SELECT is_online, availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(&online, &status)
	if err != nil {
		t.Fatalf("read BUSY presence: %v", err)
	}

	if !online || status != "BUSY" {
		t.Fatalf(
			"BUSY driver was modified: online=%v status=%s",
			online,
			status,
		)
	}

	// Other fixtures in the test database may also be eligible
	// for reconciliation, so do not assert a global transition count.
	_ = transitioned
}

func TestReconcileRegulatoryBoundedPreservesAvailableDriverWithActiveTrip(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	customerID := uuid.NewString()
	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

	defer func() {
		cleanupCtx := context.Background()

		for _, item := range []struct {
			query string
			id    string
		}{
			{"DELETE FROM trips WHERE id = $1", tripID},
			{"DELETE FROM ride_requests WHERE id = $1", rideRequestID},
			{"DELETE FROM driver_presence WHERE driver_id = $1", fixture.UserID},
			{"DELETE FROM driver_documents WHERE driver_id = $1", fixture.DriverID},
			{"DELETE FROM users WHERE id = $1", customerID},
		} {
			if _, err := db.Exec(cleanupCtx, item.query, item.id); err != nil {
				t.Errorf("cleanup %q: %v", item.query, err)
			}
		}

		if err := cleanup(cleanupCtx); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(ctx, `
		INSERT INTO users (
			id, email, password_hash,
			first_name, last_name,
			is_active, is_verified
		)
		VALUES (
			$1, $2, 'integration-test-hash',
			'Regulatory', 'Customer',
			TRUE, TRUE
		)
	`, customerID, "regulatory-"+customerID+"@example.test")
	if err != nil {
		t.Fatalf("create disposable customer: %v", err)
	}

	_, err = db.Exec(ctx, `
		UPDATE drivers
		SET driving_license_expiry = $2
		WHERE id = $1
	`, fixture.DriverID, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("expire driving licence: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			vehicle_id, assignment_id,
			is_online, availability_status,
			last_heartbeat_at
		)
		VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("create AVAILABLE presence: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO ride_requests (
			id, customer_id,
			pickup_address, pickup_latitude, pickup_longitude,
			destination_address, destination_latitude, destination_longitude,
			requested_vehicle_type, passenger_count,
			status, requested_at, expires_at
		)
		VALUES (
			$1,$2,
			'Regulatory Test Pickup',60.2055,24.6559,
			'Regulatory Test Destination',60.1719,24.9414,
			'STANDARD',1,
			'ACCEPTED',$3,$4
		)
	`, rideRequestID, customerID, now, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("create accepted ride request: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO trips (
			id, ride_request_id, customer_id,
			driver_id, vehicle_id, company_id,
			branch_id, fleet_id,
			status, assigned_at, is_active
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,
			'ASSIGNED',$9,TRUE
		)
	`,
		tripID,
		rideRequestID,
		customerID,
		fixture.UserID,
		fixture.VehicleID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.FleetID,
		now,
	)
	if err != nil {
		t.Fatalf("create active ASSIGNED trip: %v", err)
	}

	service := &Service{
		db:  db,
		cfg: cfg,
		complianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
			return driver_compliance.NewService(
				driver_compliance.Dependencies{
					Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
					Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
				},
			)
		},
	}

	_, err = service.ReconcileRegulatoryBounded(ctx, now, 10000)
	if err != nil {
		t.Fatalf("reconcile active-trip driver: %v", err)
	}

	var online bool
	var status string

	err = db.QueryRow(ctx, `
		SELECT is_online, availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(&online, &status)
	if err != nil {
		t.Fatalf("read presence after reconciliation: %v", err)
	}

	if !online || status != "AVAILABLE" {
		t.Fatalf(
			"active-trip driver was modified: online=%v status=%s",
			online, status,
		)
	}

	var tripStatus string
	var tripActive bool

	err = db.QueryRow(ctx, `
		SELECT status, is_active
		FROM trips
		WHERE id = $1
	`, tripID).Scan(&tripStatus, &tripActive)
	if err != nil {
		t.Fatalf("read protected trip: %v", err)
	}

	if tripStatus != "ASSIGNED" || !tripActive {
		t.Fatalf(
			"active trip was modified: status=%s active=%v",
			tripStatus, tripActive,
		)
	}
}

func TestReconcileRegulatoryBoundedCursorEventuallyProcessesAllDrivers(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	type fixtureEntry struct {
		driver  *testutil.DriverFixture
		cleanup func(context.Context) error
	}

	var fixtures []fixtureEntry

	defer func() {
		cleanupCtx := context.Background()

		for i := len(fixtures) - 1; i >= 0; i-- {
			entry := fixtures[i]

			_, _ = db.Exec(
				cleanupCtx,
				"DELETE FROM driver_presence WHERE driver_id = $1",
				entry.driver.UserID,
			)

			_, _ = db.Exec(
				cleanupCtx,
				"DELETE FROM driver_documents WHERE driver_id = $1",
				entry.driver.DriverID,
			)

			if err := entry.cleanup(cleanupCtx); err != nil {
				t.Errorf("cleanup driver fixture: %v", err)
			}
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	for i := 0; i < 2; i++ {
		fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
		if err != nil {
			t.Fatalf("create fixture %d: %v", i, err)
		}

		fixtures = append(fixtures, fixtureEntry{
			driver:  fixture,
			cleanup: cleanup,
		})

		_, err = db.Exec(ctx, `
			UPDATE drivers
			SET driving_license_expiry = $2
			WHERE id = $1
		`, fixture.DriverID, now.AddDate(0, 0, -1))
		if err != nil {
			t.Fatalf("expire fixture %d licence: %v", i, err)
		}

		_, err = db.Exec(ctx, `
			INSERT INTO driver_presence (
				driver_id, company_id, branch_id,
				vehicle_id, assignment_id,
				is_online, availability_status,
				last_heartbeat_at
			)
			VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
		`,
			fixture.UserID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.VehicleID,
			fixture.AssignmentID,
			now,
		)
		if err != nil {
			t.Fatalf("create fixture %d presence: %v", i, err)
		}
	}

	service := &Service{
		db:  db,
		cfg: cfg,
		complianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
			return driver_compliance.NewService(
				driver_compliance.Dependencies{
					Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
					Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
				},
			)
		},
	}

	// Batch size one exercises cursor advancement and wraparound.
	// Additional unrelated test data may exist in the database,
	// so allow sufficient cycles to traverse the candidate set.
	for cycle := 0; cycle < 1000; cycle++ {
		_, err := service.ReconcileRegulatoryBounded(ctx, now, 1)
		if err != nil {
			t.Fatalf("reconciliation cycle %d: %v", cycle, err)
		}

		allOffline := true

		for _, entry := range fixtures {
			var online bool

			err := db.QueryRow(ctx, `
				SELECT is_online
				FROM driver_presence
				WHERE driver_id = $1
			`, entry.driver.UserID).Scan(&online)
			if err != nil {
				t.Fatalf("read fixture presence: %v", err)
			}

			if online {
				allOffline = false
			}
		}

		if allOffline {
			return
		}
	}

	t.Fatal("bounded reconciliation did not process both drivers")
}

func TestReconcileRegulatoryBoundedRetriesAfterComplianceError(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		); err != nil {
			t.Errorf("cleanup presence: %v", err)
		}

		if _, err := db.Exec(cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		); err != nil {
			t.Errorf("cleanup documents: %v", err)
		}

		if err := cleanup(cleanupCtx); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = db.Exec(ctx, `
		UPDATE drivers
		SET driving_license_expiry = $2
		WHERE id = $1
	`, fixture.DriverID, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("expire driver licence: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			vehicle_id, assignment_id,
			is_online, availability_status,
			last_heartbeat_at
		)
		VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		now,
	)
	if err != nil {
		t.Fatalf("create online presence: %v", err)
	}

	injectedError := errors.New("injected regulatory compliance failure")
	failingEvaluator := &complianceEvaluatorStub{
		err: injectedError,
	}

	service := &Service{
		db:  db,
		cfg: cfg,
		complianceForDB: func(postgresrepo.DBTX) ComplianceEvaluator {
			return failingEvaluator
		},
	}

	// First pass: compliance fails after the driver and presence
	// rows have been locked. The transaction must roll back.
	_, err = service.ReconcileRegulatoryBounded(ctx, now, 10000)

	if !errors.Is(err, injectedError) {
		t.Fatalf(
			"expected injected compliance error, got %v",
			err,
		)
	}

	if failingEvaluator.calls == 0 {
		t.Fatal("injected compliance evaluator was not called")
	}

	if failingEvaluator.lastDriverID != fixture.DriverID {
		t.Fatalf(
			"compliance evaluated driver %s, expected %s",
			failingEvaluator.lastDriverID,
			fixture.DriverID,
		)
	}

	var online bool
	var status string

	err = db.QueryRow(ctx, `
		SELECT is_online, availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(&online, &status)
	if err != nil {
		t.Fatalf("read presence after compliance failure: %v", err)
	}

	if !online || status != "AVAILABLE" {
		t.Fatalf(
			"presence changed despite compliance failure: online=%v status=%s",
			online, status,
		)
	}

	// Restore the real transactional compliance evaluator.
	service.complianceForDB = func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
		return driver_compliance.NewService(
			driver_compliance.Dependencies{
				Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
				Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
			},
		)
	}

	// The cursor advanced before the failed evaluation.
	// Repeated bounded passes must wrap and revisit this driver.
	recovered := false

	for cycle := 0; cycle < 1000; cycle++ {
		_, err := service.ReconcileRegulatoryBounded(ctx, now, 1)
		if err != nil {
			t.Fatalf("retry cycle %d: %v", cycle, err)
		}

		err = db.QueryRow(ctx, `
			SELECT is_online, availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`, fixture.UserID).Scan(&online, &status)
		if err != nil {
			t.Fatalf("read presence during retry: %v", err)
		}

		if !online && status == "OFFLINE" {
			recovered = true
			break
		}
	}

	if !recovered {
		t.Fatal("failed compliance candidate was not retried after cursor wrap")
	}
}
