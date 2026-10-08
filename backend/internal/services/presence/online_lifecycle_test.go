package presence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	drivercomplianceservice "github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestGoOnlineRejectsInactiveCompany(t *testing.T) {
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
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, FALSE, 'OFFLINE', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	// Mutate the isolated parent directly. GoOnline must re-read lifecycle
	// state transactionally rather than trusting the active assignment.
	if _, err := db.Exec(
		ctx,
		`UPDATE companies SET is_active = FALSE WHERE id = $1`,
		fixture.CompanyID,
	); err != nil {
		t.Fatalf("deactivate fixture company: %v", err)
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance: &complianceEvaluatorStub{
				eligible: true,
			},
		},
	)

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: fixture.UserID,
		},
	)

	if !errors.Is(err, ErrCompanyInactive) {
		t.Fatalf("expected ErrCompanyInactive, got %v", err)
	}

	var (
		isOnline           bool
		availabilityStatus string
		assignmentID       *string
		vehicleID          *string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				assignment_id,
				vehicle_id
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&assignmentID,
		&vehicleID,
	); err != nil {
		t.Fatalf("load presence after rejected GoOnline: %v", err)
	}

	if isOnline {
		t.Fatal("expected driver to remain offline")
	}
	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected availability %s, got %s",
			StatusOffline,
			availabilityStatus,
		)
	}
	if assignmentID != nil {
		t.Fatalf("expected assignment to remain detached, got %s", *assignmentID)
	}
	if vehicleID != nil {
		t.Fatalf("expected vehicle to remain detached, got %s", *vehicleID)
	}
}

func TestGoOnlineRejectsComplianceRevokedWhileWaitingForDriverAggregateLock(
	t *testing.T,
) {
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

	ctx := context.Background()

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	cleanupCompliance, err :=
		testutil.MakeDriverRegulatorilyCompliant(ctx, db, fixture)
	if err != nil {
		t.Fatalf("make driver regulatorily compliant: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupCompliance(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver compliance fixture: %v", cleanupErr)
		}
	}()

	if _, err := db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				availability_status,
				is_online
			)
			VALUES ($1, $2, $3, $4, FALSE)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		StatusOffline,
	); err != nil {
		t.Fatalf("insert driver presence: %v", err)
	}

	documentRepo := postgresrepo.NewDriverDocumentRepository(db)

	drivingLicense, err := documentRepo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		models.DriverDocumentTypeDrivingLicense,
	)
	if err != nil {
		t.Fatalf("get verified driving-license document: %v", err)
	}

	revokeTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin revocation transaction: %v", err)
	}

	revokeTxFinished := false
	defer func() {
		if !revokeTxFinished {
			_ = revokeTx.Rollback(context.Background())
		}
	}()

	if _, err := revokeTx.Exec(
		ctx,
		`
			SELECT id
			FROM drivers
			WHERE id = $1
			FOR UPDATE
		`,
		fixture.DriverID,
	); err != nil {
		t.Fatalf("lock driver aggregate for revocation: %v", err)
	}

	if _, err := revokeTx.Exec(
		ctx,
		`
			UPDATE driver_documents
			SET
				status = $2,
				revoked_at = NOW(),
				revoked_by_user_id = $3,
				revocation_reason = $4,
				updated_at = NOW()
			WHERE id = $1
			  AND deleted_at IS NULL
		`,
		drivingLicense.ID,
		models.DriverDocumentStatusRevoked,
		fixture.UserID,
		"concurrent regulatory revocation",
	); err != nil {
		t.Fatalf("stage driving-license revocation: %v", err)
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  &complianceEvaluatorStub{eligible: true},
			ComplianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
				return drivercomplianceservice.NewService(
					drivercomplianceservice.Dependencies{
						Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
						Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
					},
				)
			},
		},
	)

	started := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		close(started)
		result <- service.GoOnline(
			context.Background(),
			GoOnlineRequest{
				UserID: fixture.UserID,
			},
		)
	}()

	<-started

	select {
	case early := <-result:
		t.Fatalf(
			"GoOnline completed while regulatory mutation held driver aggregate lock: %v",
			early,
		)
	case <-time.After(150 * time.Millisecond):
		// Expected: GoOnline is blocked acquiring the authoritative driver row.
	}

	if err := revokeTx.Commit(ctx); err != nil {
		t.Fatalf("commit regulatory revocation: %v", err)
	}
	revokeTxFinished = true

	select {
	case goOnlineErr := <-result:
		if !errors.Is(goOnlineErr, ErrDriverComplianceRequired) {
			t.Fatalf(
				"expected ErrDriverComplianceRequired after committed revocation, got %v",
				goOnlineErr,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GoOnline did not complete after regulatory revocation committed")
	}

	var (
		isOnline           bool
		availabilityStatus string
		assignmentID       *string
		vehicleID          *string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				assignment_id,
				vehicle_id
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&assignmentID,
		&vehicleID,
	); err != nil {
		t.Fatalf("load presence after rejected GoOnline: %v", err)
	}

	if isOnline {
		t.Fatal("expected driver to remain offline after regulatory revocation")
	}
	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected availability %s, got %s",
			StatusOffline,
			availabilityStatus,
		)
	}
	if assignmentID != nil {
		t.Fatalf("expected assignment to remain detached, got %s", *assignmentID)
	}
	if vehicleID != nil {
		t.Fatalf("expected vehicle to remain detached, got %s", *vehicleID)
	}
}

func TestGoOnlineRejectsInactiveBranch(t *testing.T) {
	testGoOnlineRejectsLifecycleMutation(
		t,
		"UPDATE branches SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.BranchID },
		ErrBranchInactive,
	)
}

func TestGoOnlineRejectsInactiveFleet(t *testing.T) {
	testGoOnlineRejectsLifecycleMutation(
		t,
		"UPDATE fleets SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.FleetID },
		ErrFleetInactive,
	)
}

func TestGoOnlineRejectsInactiveVehicle(t *testing.T) {
	testGoOnlineRejectsLifecycleMutation(
		t,
		"UPDATE vehicles SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.VehicleID },
		ErrVehicleInactive,
	)
}

func TestGoOnlineRejectsInactiveDriver(t *testing.T) {
	testGoOnlineRejectsLifecycleMutation(
		t,
		"UPDATE drivers SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.DriverID },
		ErrDriverInactive,
	)
}

func testGoOnlineRejectsLifecycleMutation(
	t *testing.T,
	updateSQL string,
	targetID func(*testutil.DriverFixture) string,
	wantErr error,
) {
	t.Helper()

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
		t.Fatalf("load config: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer cleanup(ctx)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				availability_status,
				is_online
			)
			VALUES ($1, $2, $3, $4, FALSE)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		StatusOffline,
	)
	if err != nil {
		t.Fatalf("insert driver presence: %v", err)
	}

	if _, err := db.Exec(
		ctx,
		updateSQL,
		targetID(fixture),
	); err != nil {
		t.Fatalf("mutate lifecycle target: %v", err)
	}

	service := &Service{
		db:          db,
		drivers:     postgresrepo.NewDriverRepository(db),
		presence:    postgresrepo.NewDriverPresenceRepository(db),
		assignments: postgresrepo.NewDriverAssignmentRepository(db),
		compliance:  &complianceEvaluatorStub{eligible: true},
	}

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: fixture.UserID,
		},
	)

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}

	presence, err :=
		postgresrepo.NewDriverPresenceRepository(db).
			GetByDriverID(ctx, fixture.UserID)
	if err != nil {
		t.Fatalf("get driver presence: %v", err)
	}

	if presence.IsOnline {
		t.Fatal("expected driver to remain offline")
	}

	if presence.AvailabilityStatus != StatusOffline {
		t.Fatalf(
			"expected OFFLINE availability, got %q",
			presence.AvailabilityStatus,
		)
	}

	if presence.AssignmentID != nil {
		t.Fatalf(
			"expected assignment to remain detached, got %v",
			*presence.AssignmentID,
		)
	}

	if presence.VehicleID != nil {
		t.Fatalf(
			"expected vehicle to remain detached, got %v",
			*presence.VehicleID,
		)
	}
}

func TestGoOnlineRejectsUnverifiedDriver(t *testing.T) {
	testGoOnlineRejectsLifecycleMutation(
		t,
		"UPDATE drivers SET is_verified = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.DriverID },
		ErrDriverInactive,
	)
}

func TestGoOnlineRejectsNonActiveDriverStatus(t *testing.T) {
	testGoOnlineRejectsLifecycleMutation(
		t,
		"UPDATE drivers SET status = 'PENDING_VERIFICATION' WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.DriverID },
		ErrDriverInactive,
	)
}

func TestGoOnlineRechecksCompanyLifecycleAfterLock(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, FALSE, 'OFFLINE', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	lifecycleTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin company lifecycle transaction: %v", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = lifecycleTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		lifecycleTx,
		"company:"+fixture.CompanyID,
	); err != nil {
		t.Fatalf("acquire company lifecycle lock: %v", err)
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance: &complianceEvaluatorStub{
				eligible: true,
			},
		},
	)

	goOnlineResult := make(chan error, 1)
	go func() {
		goOnlineResult <- service.GoOnline(
			context.Background(),
			GoOnlineRequest{
				UserID: fixture.UserID,
			},
		)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_locks
				WHERE locktype = 'advisory'
				  AND granted = FALSE
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting > 0 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("GoOnline never waited on company lifecycle advisory lock")
		}

		time.Sleep(10 * time.Millisecond)
	}

	_, err = lifecycleTx.Exec(
		ctx,
		`
			UPDATE companies
			SET is_active = FALSE, updated_at = NOW()
			WHERE id = $1
			  AND deleted_at IS NULL
		`,
		fixture.CompanyID,
	)
	if err != nil {
		t.Fatalf("deactivate company while holding lifecycle lock: %v", err)
	}

	if err := lifecycleTx.Commit(ctx); err != nil {
		t.Fatalf("commit company deactivation: %v", err)
	}
	committed = true

	select {
	case err := <-goOnlineResult:
		if !errors.Is(err, ErrCompanyInactive) {
			t.Fatalf(
				"expected ErrCompanyInactive after company lifecycle won serialization, got %v",
				err,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GoOnline did not finish after company lifecycle lock released")
	}

	var (
		isOnline           bool
		availabilityStatus string
		assignmentID       *string
		vehicleID          *string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				assignment_id,
				vehicle_id
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&assignmentID,
		&vehicleID,
	)
	if err != nil {
		t.Fatalf("load presence after company lifecycle race: %v", err)
	}

	if isOnline {
		t.Fatal("expected driver to remain offline")
	}
	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected availability %s, got %s",
			StatusOffline,
			availabilityStatus,
		)
	}
	if assignmentID != nil {
		t.Fatalf("expected assignment to remain detached, got %s", *assignmentID)
	}
	if vehicleID != nil {
		t.Fatalf("expected vehicle to remain detached, got %s", *vehicleID)
	}
}

func TestGoOnlineRejectsAssignmentClosedWhileWaitingForLifecycleLock(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, FALSE, 'OFFLINE', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	lifecycleTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin company lifecycle transaction: %v", err)
	}

	released := false
	defer func() {
		if !released {
			_ = lifecycleTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		lifecycleTx,
		"company:"+fixture.CompanyID,
	); err != nil {
		t.Fatalf("acquire company lifecycle lock: %v", err)
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance: &complianceEvaluatorStub{
				eligible: true,
			},
		},
	)

	goOnlineResult := make(chan error, 1)
	go func() {
		goOnlineResult <- service.GoOnline(
			context.Background(),
			GoOnlineRequest{
				UserID: fixture.UserID,
			},
		)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_locks
				WHERE locktype = 'advisory'
				  AND granted = FALSE
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting > 0 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("GoOnline never waited on company lifecycle advisory lock")
		}

		time.Sleep(10 * time.Millisecond)
	}

	// GoOnline has already pre-read this assignment and is now blocked before
	// taking the presence lock. Cleanup/unassignment must remain able to close
	// it without acquiring parent lifecycle locks.
	tag, err := db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET unassigned_at = NOW(), updated_at = NOW()
			WHERE id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("close assignment while GoOnline waits: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf(
			"expected to close one active assignment, closed %d",
			tag.RowsAffected(),
		)
	}

	if err := lifecycleTx.Rollback(ctx); err != nil {
		t.Fatalf("release company lifecycle lock: %v", err)
	}
	released = true

	select {
	case err := <-goOnlineResult:
		if !errors.Is(err, ErrDriverAssignmentRequired) {
			t.Fatalf(
				"expected ErrDriverAssignmentRequired after assignment closed, got %v",
				err,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GoOnline did not finish after lifecycle lock released")
	}

	var (
		isOnline           bool
		availabilityStatus string
		assignmentID       *string
		vehicleID          *string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				assignment_id,
				vehicle_id
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&assignmentID,
		&vehicleID,
	)
	if err != nil {
		t.Fatalf("load presence after stale assignment race: %v", err)
	}

	if isOnline {
		t.Fatal("expected driver to remain offline")
	}
	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected availability %s, got %s",
			StatusOffline,
			availabilityStatus,
		)
	}
	if assignmentID != nil {
		t.Fatalf("expected assignment to remain detached, got %s", *assignmentID)
	}
	if vehicleID != nil {
		t.Fatalf("expected vehicle to remain detached, got %s", *vehicleID)
	}
}

func TestUpdateAvailabilityRejectsAvailableWhenCompanyInactive(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, 'BREAK', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
	)
	if err != nil {
		t.Fatalf("create online break presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	_, err = db.Exec(
		ctx,
		`
			UPDATE companies
			SET is_active = FALSE, updated_at = NOW()
			WHERE id = $1
			  AND deleted_at IS NULL
		`,
		fixture.CompanyID,
	)
	if err != nil {
		t.Fatalf("deactivate company: %v", err)
	}

	service := NewService(
		Dependencies{
			DB: db,
		},
	)

	err = service.UpdateAvailability(
		ctx,
		UpdateAvailabilityRequest{
			UserID: fixture.UserID,
			Status: StatusAvailable,
		},
	)
	if !errors.Is(err, ErrCompanyInactive) {
		t.Fatalf(
			"expected ErrCompanyInactive when becoming available, got %v",
			err,
		)
	}

	var status string
	err = db.QueryRow(
		ctx,
		`
			SELECT availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("load presence after rejected availability update: %v", err)
	}

	if status != StatusBreak {
		t.Fatalf(
			"expected rejected transition to preserve %s, got %s",
			StatusBreak,
			status,
		)
	}
}

func TestUpdateAvailabilityRechecksCompanyLifecycleAfterLock(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, 'BREAK', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
	)
	if err != nil {
		t.Fatalf("create online break presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	lifecycleTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin company lifecycle transaction: %v", err)
	}

	released := false
	defer func() {
		if !released {
			_ = lifecycleTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		lifecycleTx,
		"company:"+fixture.CompanyID,
	); err != nil {
		t.Fatalf("acquire company lifecycle lock: %v", err)
	}

	service := NewService(Dependencies{DB: db})

	result := make(chan error, 1)
	go func() {
		result <- service.UpdateAvailability(
			context.Background(),
			UpdateAvailabilityRequest{
				UserID: fixture.UserID,
				Status: StatusAvailable,
			},
		)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_locks
				WHERE locktype = 'advisory'
				  AND granted = FALSE
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("UpdateAvailability never waited on company lifecycle lock")
		}

		time.Sleep(10 * time.Millisecond)
	}

	_, err = lifecycleTx.Exec(
		ctx,
		`
			UPDATE companies
			SET is_active = FALSE, updated_at = NOW()
			WHERE id = $1
			  AND deleted_at IS NULL
		`,
		fixture.CompanyID,
	)
	if err != nil {
		t.Fatalf("deactivate company while availability update waits: %v", err)
	}

	if err := lifecycleTx.Commit(ctx); err != nil {
		t.Fatalf("commit company deactivation: %v", err)
	}
	released = true

	select {
	case err := <-result:
		if !errors.Is(err, ErrCompanyInactive) {
			t.Fatalf(
				"expected ErrCompanyInactive after concurrent company deactivation, got %v",
				err,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateAvailability did not finish after lifecycle lock released")
	}

	var (
		isOnline bool
		status   string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT is_online, availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(&isOnline, &status)
	if err != nil {
		t.Fatalf("load presence after rejected availability update: %v", err)
	}

	if !isOnline {
		t.Fatal("expected rejected AVAILABLE transition to preserve online state")
	}
	if status != StatusBreak {
		t.Fatalf(
			"expected rejected transition to preserve %s, got %s",
			StatusBreak,
			status,
		)
	}
}

func TestUpdateAvailabilityRejectsAssignmentClosedWhileWaitingForLifecycleLock(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, 'BREAK', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
	)
	if err != nil {
		t.Fatalf("create online break presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	lifecycleTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin company lifecycle transaction: %v", err)
	}

	released := false
	defer func() {
		if !released {
			_ = lifecycleTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		lifecycleTx,
		"company:"+fixture.CompanyID,
	); err != nil {
		t.Fatalf("acquire company lifecycle lock: %v", err)
	}

	service := NewService(Dependencies{DB: db})

	result := make(chan error, 1)
	go func() {
		result <- service.UpdateAvailability(
			context.Background(),
			UpdateAvailabilityRequest{
				UserID: fixture.UserID,
				Status: StatusAvailable,
			},
		)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_locks
				WHERE locktype = 'advisory'
				  AND granted = FALSE
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("UpdateAvailability never waited on company lifecycle lock")
		}

		time.Sleep(10 * time.Millisecond)
	}

	tag, err := db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET unassigned_at = NOW(), updated_at = NOW()
			WHERE id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("close assignment while availability update waits: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected to close one assignment, closed %d", tag.RowsAffected())
	}

	if err := lifecycleTx.Rollback(ctx); err != nil {
		t.Fatalf("release company lifecycle lock: %v", err)
	}
	released = true

	select {
	case err := <-result:
		if !errors.Is(err, ErrDriverAssignmentRequired) {
			t.Fatalf(
				"expected ErrDriverAssignmentRequired after assignment closed, got %v",
				err,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateAvailability did not finish after lifecycle lock released")
	}

	var status string
	err = db.QueryRow(
		ctx,
		`
			SELECT availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("load presence after stale assignment race: %v", err)
	}

	if status != StatusBreak {
		t.Fatalf(
			"expected rejected transition to preserve %s, got %s",
			StatusBreak,
			status,
		)
	}
}

func TestUpdateAvailabilityRejectsAvailableWhenBranchInactive(t *testing.T) {
	testUpdateAvailabilityRejectsLifecycleMutation(
		t,
		"UPDATE branches SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.BranchID },
		ErrBranchInactive,
	)
}

func TestUpdateAvailabilityRejectsAvailableWhenFleetInactive(t *testing.T) {
	testUpdateAvailabilityRejectsLifecycleMutation(
		t,
		"UPDATE fleets SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.FleetID },
		ErrFleetInactive,
	)
}

func TestUpdateAvailabilityRejectsAvailableWhenVehicleInactive(t *testing.T) {
	testUpdateAvailabilityRejectsLifecycleMutation(
		t,
		"UPDATE vehicles SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.VehicleID },
		ErrVehicleInactive,
	)
}

func TestUpdateAvailabilityRejectsAvailableWhenDriverInactive(t *testing.T) {
	testUpdateAvailabilityRejectsLifecycleMutation(
		t,
		"UPDATE drivers SET is_active = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.DriverID },
		ErrDriverInactive,
	)
}

func TestUpdateAvailabilityRejectsAvailableWhenDriverUnverified(t *testing.T) {
	testUpdateAvailabilityRejectsLifecycleMutation(
		t,
		"UPDATE drivers SET is_verified = FALSE WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.DriverID },
		ErrDriverInactive,
	)
}

func TestUpdateAvailabilityRejectsAvailableWhenDriverStatusNonActive(t *testing.T) {
	testUpdateAvailabilityRejectsLifecycleMutation(
		t,
		"UPDATE drivers SET status = 'PENDING_VERIFICATION' WHERE id = $1",
		func(f *testutil.DriverFixture) string { return f.DriverID },
		ErrDriverInactive,
	)
}

func testUpdateAvailabilityRejectsLifecycleMutation(
	t *testing.T,
	updateSQL string,
	targetID func(*testutil.DriverFixture) string,
	wantErr error,
) {
	t.Helper()

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

	ctx := context.Background()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, 'BREAK', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
	)
	if err != nil {
		t.Fatalf("create online break presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	if _, err := db.Exec(
		ctx,
		updateSQL,
		targetID(fixture),
	); err != nil {
		t.Fatalf("mutate lifecycle target: %v", err)
	}

	service := NewService(Dependencies{DB: db})

	err = service.UpdateAvailability(
		ctx,
		UpdateAvailabilityRequest{
			UserID: fixture.UserID,
			Status: StatusAvailable,
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}

	var (
		isOnline bool
		status   string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT is_online, availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(&isOnline, &status)
	if err != nil {
		t.Fatalf("load presence after rejected availability update: %v", err)
	}

	if !isOnline {
		t.Fatal("expected rejected AVAILABLE transition to preserve online state")
	}
	if status != StatusBreak {
		t.Fatalf(
			"expected rejected transition to preserve %s, got %s",
			StatusBreak,
			status,
		)
	}
}

func TestUpdateAvailabilityDoesNotBringOfflineDriverOnline(t *testing.T) {
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

	ctx := context.Background()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, FALSE, 'OFFLINE', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	service := NewService(Dependencies{DB: db})

	for _, status := range []string{
		StatusAvailable,
		StatusBreak,
		StatusOffDuty,
	} {
		t.Run(status, func(t *testing.T) {
			err := service.UpdateAvailability(
				ctx,
				UpdateAvailabilityRequest{
					UserID: fixture.UserID,
					Status: status,
				},
			)
			if err == nil {
				t.Fatalf(
					"expected offline driver transition to %s to be rejected",
					status,
				)
			}

			var (
				isOnline           bool
				availabilityStatus string
			)

			err = db.QueryRow(
				ctx,
				`
					SELECT is_online, availability_status
					FROM driver_presence
					WHERE driver_id = $1
				`,
				fixture.UserID,
			).Scan(&isOnline, &availabilityStatus)
			if err != nil {
				t.Fatalf("load presence after rejected transition: %v", err)
			}

			if isOnline {
				t.Fatalf(
					"UpdateAvailability(%s) brought offline driver online",
					status,
				)
			}
			if availabilityStatus != StatusOffline {
				t.Fatalf(
					"expected rejected transition to preserve %s, got %s",
					StatusOffline,
					availabilityStatus,
				)
			}
		})
	}
}

func TestUpdateAvailabilityRejectsComplianceRevokedWhileWaitingForDriverAggregateLock(
	t *testing.T,
) {
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

	ctx := context.Background()

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	cleanupCompliance, err :=
		testutil.MakeDriverRegulatorilyCompliant(ctx, db, fixture)
	if err != nil {
		t.Fatalf("make driver regulatorily compliant: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupCompliance(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver compliance fixture: %v", cleanupErr)
		}
	}()

	if _, err := db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, $6, NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
		StatusBreak,
	); err != nil {
		t.Fatalf("create online break presence: %v", err)
	}

	documentRepo := postgresrepo.NewDriverDocumentRepository(db)

	drivingLicense, err := documentRepo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		models.DriverDocumentTypeDrivingLicense,
	)
	if err != nil {
		t.Fatalf("get verified driving-license document: %v", err)
	}

	revokeTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin revocation transaction: %v", err)
	}

	revokeTxFinished := false
	defer func() {
		if !revokeTxFinished {
			_ = revokeTx.Rollback(context.Background())
		}
	}()

	if _, err := revokeTx.Exec(
		ctx,
		`
			SELECT id
			FROM drivers
			WHERE id = $1
			FOR UPDATE
		`,
		fixture.DriverID,
	); err != nil {
		t.Fatalf("lock driver aggregate for revocation: %v", err)
	}

	if _, err := revokeTx.Exec(
		ctx,
		`
			UPDATE driver_documents
			SET
				status = $2,
				revoked_at = NOW(),
				revoked_by_user_id = $3,
				revocation_reason = $4,
				updated_at = NOW()
			WHERE id = $1
			  AND deleted_at IS NULL
		`,
		drivingLicense.ID,
		models.DriverDocumentStatusRevoked,
		fixture.UserID,
		"concurrent regulatory revocation",
	); err != nil {
		t.Fatalf("stage driving-license revocation: %v", err)
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  &complianceEvaluatorStub{eligible: true},
			ComplianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
				return drivercomplianceservice.NewService(
					drivercomplianceservice.Dependencies{
						Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
						Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
					},
				)
			},
		},
	)

	started := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		close(started)
		result <- service.UpdateAvailability(
			context.Background(),
			UpdateAvailabilityRequest{
				UserID: fixture.UserID,
				Status: StatusAvailable,
			},
		)
	}()

	<-started

	select {
	case early := <-result:
		t.Fatalf(
			"UpdateAvailability completed while regulatory mutation held driver aggregate lock: %v",
			early,
		)
	case <-time.After(150 * time.Millisecond):
		// Expected: AVAILABLE is blocked acquiring the authoritative driver row.
	}

	if err := revokeTx.Commit(ctx); err != nil {
		t.Fatalf("commit regulatory revocation: %v", err)
	}
	revokeTxFinished = true

	select {
	case availabilityErr := <-result:
		if !errors.Is(availabilityErr, ErrDriverComplianceRequired) {
			t.Fatalf(
				"expected ErrDriverComplianceRequired after committed revocation, got %v",
				availabilityErr,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateAvailability did not complete after regulatory revocation committed")
	}

	var (
		isOnline           bool
		availabilityStatus string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT is_online, availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(&isOnline, &availabilityStatus); err != nil {
		t.Fatalf("load presence after rejected AVAILABLE transition: %v", err)
	}

	if !isOnline {
		t.Fatal("expected rejected AVAILABLE transition to preserve online state")
	}
	if availabilityStatus != StatusBreak {
		t.Fatalf(
			"expected rejected AVAILABLE transition to preserve %s, got %s",
			StatusBreak,
			availabilityStatus,
		)
	}
}

func TestUpdateAvailabilityRejectsAvailableWhenDriverNonCompliant(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				assignment_id,
				vehicle_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, TRUE, 'BREAK', NOW())
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
	)
	if err != nil {
		t.Fatalf("create online break presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	compliance := &complianceEvaluatorStub{
		eligible: false,
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  compliance,
		},
	)

	err = service.UpdateAvailability(
		ctx,
		UpdateAvailabilityRequest{
			UserID: fixture.UserID,
			Status: StatusAvailable,
		},
	)

	if !errors.Is(err, ErrDriverComplianceRequired) {
		t.Fatalf(
			"expected ErrDriverComplianceRequired, got %v",
			err,
		)
	}

	if compliance.calls != 1 {
		t.Fatalf(
			"expected one compliance evaluation, got %d",
			compliance.calls,
		)
	}

	if compliance.lastDriverID != fixture.DriverID {
		t.Fatalf(
			"expected compliance evaluation for driver %s, got %s",
			fixture.DriverID,
			compliance.lastDriverID,
		)
	}

	var (
		isOnline bool
		status   string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT is_online, availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(&isOnline, &status)
	if err != nil {
		t.Fatalf("load presence after rejected AVAILABLE transition: %v", err)
	}

	if !isOnline {
		t.Fatal("expected rejected AVAILABLE transition to preserve online state")
	}

	if status != StatusBreak {
		t.Fatalf(
			"expected rejected AVAILABLE transition to preserve %s, got %s",
			StatusBreak,
			status,
		)
	}
}

func TestUpdateAvailabilityDisablingTransitionsDoNotWaitForParentLifecycleLock(t *testing.T) {
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

	for _, status := range []string{
		StatusBreak,
		StatusOffDuty,
	} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()

			fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
			if err != nil {
				t.Fatalf("create driver fixture: %v", err)
			}
			defer func() {
				if err := cleanup(context.Background()); err != nil {
					t.Logf("cleanup driver fixture: %v", err)
				}
			}()

			_, err = db.Exec(
				ctx,
				`
					INSERT INTO driver_presence (
						driver_id,
						company_id,
						branch_id,
						assignment_id,
						vehicle_id,
						is_online,
						availability_status,
						updated_at
					)
					VALUES ($1, $2, $3, $4, $5, TRUE, 'AVAILABLE', NOW())
				`,
				fixture.UserID,
				fixture.CompanyID,
				fixture.BranchID,
				fixture.AssignmentID,
				fixture.VehicleID,
			)
			if err != nil {
				t.Fatalf("create available presence: %v", err)
			}
			defer func() {
				_, _ = db.Exec(
					context.Background(),
					`DELETE FROM driver_presence WHERE driver_id = $1`,
					fixture.UserID,
				)
			}()

			lifecycleTx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin company lifecycle transaction: %v", err)
			}

			released := false
			defer func() {
				if !released {
					_ = lifecycleTx.Rollback(context.Background())
				}
			}()

			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				lifecycleTx,
				"company:"+fixture.CompanyID,
			); err != nil {
				t.Fatalf("acquire company lifecycle lock: %v", err)
			}

			service := NewService(Dependencies{DB: db})

			result := make(chan error, 1)
			go func() {
				result <- service.UpdateAvailability(
					context.Background(),
					UpdateAvailabilityRequest{
						UserID: fixture.UserID,
						Status: status,
					},
				)
			}()

			select {
			case err := <-result:
				if err != nil {
					t.Fatalf(
						"expected disabling transition to %s to succeed while parent lifecycle lock is held, got %v",
						status,
						err,
					)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf(
					"disabling transition to %s waited for parent lifecycle lock",
					status,
				)
			}

			if err := lifecycleTx.Rollback(ctx); err != nil {
				t.Fatalf("release company lifecycle lock: %v", err)
			}
			released = true

			var (
				isOnline           bool
				availabilityStatus string
			)

			err = db.QueryRow(
				ctx,
				`
					SELECT is_online, availability_status
					FROM driver_presence
					WHERE driver_id = $1
				`,
				fixture.UserID,
			).Scan(&isOnline, &availabilityStatus)
			if err != nil {
				t.Fatalf("load presence after disabling transition: %v", err)
			}

			if !isOnline {
				t.Fatal("expected disabling availability transition to preserve online state")
			}
			if availabilityStatus != status {
				t.Fatalf(
					"expected availability %s, got %s",
					status,
					availabilityStatus,
				)
			}
		})
	}
}
