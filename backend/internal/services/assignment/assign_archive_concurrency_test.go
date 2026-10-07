package assignment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestAssignWaitsForVehicleArchiveAndThenRejectsArchivedVehicle(
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
	t.Cleanup(func() {
		db.Close()
	})

	fixture, cleanup, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	// Return the fixture driver and vehicle to an unassigned state.
	//
	// The fixture cleanup deletes this same assignment row by ID, so closing
	// it rather than replacing it preserves cleanup ownership.
	_, err = db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET
				unassigned_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	// Assignment requires a lockable presence row. OFFLINE is intentionally
	// sufficient: assignment ownership and online availability are separate
	// lifecycle concerns.
	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status
			)
			VALUES (
				$1,
				$2,
				$3,
				FALSE,
				'OFFLINE'
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create fixture driver presence: %v", err)
	}

	archiveTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin archive transaction: %v", err)
	}

	archiveCommitted := false
	defer func() {
		if !archiveCommitted {
			_ = archiveTx.Rollback(context.Background())
		}
	}()

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		archiveTx,
		"vehicle:"+fixture.VehicleID,
	); err != nil {
		t.Fatalf("acquire archive vehicle lock: %v", err)
	}

	assignService := NewService(
		Dependencies{
			DB: db,
		},
	)

	assignResult := make(chan error, 1)

	go func() {
		_, err := assignService.Assign(
			context.Background(),
			fixture.UserID,
			AssignDriverRequest{
				VehicleID: fixture.VehicleID,
				Notes:     "archive concurrency integration test",
			},
		)

		assignResult <- err
	}()

	// Do not use a timing-only assertion to infer that Assign reached the
	// advisory lock. PostgreSQL exposes ungranted advisory locks directly.
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
			t.Fatalf(
				"inspect advisory lock waiters: %v",
				err,
			)
		}

		if waiting > 0 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal(
				"assignment never waited on vehicle advisory lock",
			)
		}

		time.Sleep(10 * time.Millisecond)
	}

	// Archive while owning the same lifecycle serialization lock that
	// Assign is waiting to acquire.
	archiveVehicles :=
		postgresrepo.NewVehicleRepositoryWithDB(
			archiveTx,
		)

	if err := archiveVehicles.Archive(
		ctx,
		fixture.VehicleID,
	); err != nil {
		t.Fatalf(
			"archive vehicle while holding lifecycle lock: %v",
			err,
		)
	}

	if err := archiveTx.Commit(ctx); err != nil {
		t.Fatalf("commit vehicle archive: %v", err)
	}
	archiveCommitted = true

	// After the archive commits, Assign acquires the vehicle lock and performs
	// its authoritative vehicle read. It must observe the archived state.
	select {
	case err := <-assignResult:
		if !errors.Is(
			err,
			ErrVehicleNotFound,
		) {
			t.Fatalf(
				"expected ErrVehicleNotFound after archive won serialization, got %v",
				err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"assignment did not finish after archive released vehicle lock",
		)
	}

	var activeAssignments int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE vehicle_id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.VehicleID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf(
			"count active assignments after archive race: %v",
			err,
		)
	}

	if activeAssignments != 0 {
		t.Fatalf(
			"archived vehicle has %d active assignments; expected 0",
			activeAssignments,
		)
	}

	var archived bool

	err = db.QueryRow(
		ctx,
		`
			SELECT deleted_at IS NOT NULL
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(&archived)
	if err != nil {
		t.Fatalf(
			"inspect vehicle after archive race: %v",
			err,
		)
	}

	if !archived {
		t.Fatal(
			"vehicle must remain archived after winning serialization",
		)
	}
}

// Keep pgx referenced here because the test deliberately exercises a real
// PostgreSQL transaction boundary rather than a repository stub.
var _ pgx.Tx
var _ = repository.ErrNotFound

func TestAssignWaitsForDriverArchiveAndThenRejectsArchivedDriver(
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
	t.Cleanup(func() {
		db.Close()
	})

	fixture, cleanup, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	// Return the fixture driver and vehicle to an unassigned state.
	//
	// Closing the fixture-owned assignment preserves cleanup ownership while
	// making the driver eligible for archival.
	_, err = db.Exec(
		ctx,
		`
			UPDATE driver_assignments
			SET
				unassigned_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("close fixture assignment: %v", err)
	}

	// Assign requires a presence row after it acquires the driver lifecycle
	// lock. Keep that row offline and idle so no unrelated lifecycle rule
	// determines the result.
	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status
			)
			VALUES (
				$1,
				$2,
				$3,
				FALSE,
				'OFFLINE'
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create fixture driver presence: %v", err)
	}

	archiveTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin driver archive transaction: %v", err)
	}

	archiveCommitted := false
	defer func() {
		if !archiveCommitted {
			_ = archiveTx.Rollback(context.Background())
		}
	}()

	archiveDrivers :=
		postgresrepo.NewDriverRepositoryWithDB(
			archiveTx,
		)

	// Acquire the exact driver-row lock used by both driver archival and
	// assignment startup.
	lockedDriver, err :=
		archiveDrivers.GetByUserIDForUpdate(
			ctx,
			fixture.UserID,
		)
	if err != nil {
		t.Fatalf("lock driver for archive: %v", err)
	}

	if lockedDriver.ID != fixture.DriverID {
		t.Fatalf(
			"locked driver %s; expected %s",
			lockedDriver.ID,
			fixture.DriverID,
		)
	}

	assignService := NewService(
		Dependencies{
			DB: db,
		},
	)

	assignResult := make(chan error, 1)

	go func() {
		_, err := assignService.Assign(
			context.Background(),
			fixture.UserID,
			AssignDriverRequest{
				VehicleID: fixture.VehicleID,
				Notes:     "driver archive concurrency integration test",
			},
		)

		assignResult <- err
	}()

	// Prove Assign is blocked on a PostgreSQL tuple/transaction dependency
	// created by the driver FOR UPDATE lock. Do not infer this from sleep
	// timing alone.
	deadline := time.Now().Add(5 * time.Second)

	for {
		var waiting int

		err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND wait_event_type = 'Lock'
				  AND state = 'active'
				  AND query LIKE '%FROM drivers%'
				  AND query LIKE '%FOR UPDATE%'
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf(
				"inspect driver row lock waiters: %v",
				err,
			)
		}

		if waiting > 0 {
			break
		}

		select {
		case err := <-assignResult:
			t.Fatalf(
				"assignment finished before driver archive released lifecycle lock: %v",
				err,
			)
		default:
		}

		if time.Now().After(deadline) {
			t.Fatal(
				"assignment never waited on driver lifecycle row lock",
			)
		}

		time.Sleep(10 * time.Millisecond)
	}

	// Archive while owning the same driver lifecycle row lock that Assign is
	// waiting to acquire.
	if err := archiveDrivers.Delete(
		ctx,
		fixture.DriverID,
	); err != nil {
		t.Fatalf(
			"archive driver while holding lifecycle lock: %v",
			err,
		)
	}

	if err := archiveTx.Commit(ctx); err != nil {
		t.Fatalf("commit driver archive: %v", err)
	}
	archiveCommitted = true

	// Once the archive commits, Assign acquires the driver row lock and its
	// authoritative active-driver lookup must observe that the profile is
	// archived.
	select {
	case err := <-assignResult:
		if !errors.Is(
			err,
			ErrDriverNotFound,
		) {
			t.Fatalf(
				"expected ErrDriverNotFound after driver archive won serialization, got %v",
				err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"assignment did not finish after driver archive released lifecycle lock",
		)
	}

	var activeAssignments int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.UserID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf(
			"count active assignments after driver archive race: %v",
			err,
		)
	}

	if activeAssignments != 0 {
		t.Fatalf(
			"archived driver has %d active assignments; expected 0",
			activeAssignments,
		)
	}

	var archived bool

	err = db.QueryRow(
		ctx,
		`
			SELECT deleted_at IS NOT NULL
			FROM drivers
			WHERE id = $1
		`,
		fixture.DriverID,
	).Scan(&archived)
	if err != nil {
		t.Fatalf(
			"inspect driver after archive race: %v",
			err,
		)
	}

	if !archived {
		t.Fatal(
			"driver must remain archived after winning serialization",
		)
	}
}

func TestAssignWaitsForFleetLifecycleAndThenRejectsInactiveFleet(
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
	t.Cleanup(func() {
		db.Close()
	})

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	_, err = db.Exec(
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
		t.Fatalf("close fixture assignment: %v", err)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status
			)
			VALUES ($1, $2, $3, FALSE, 'OFFLINE')
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create fixture driver presence: %v", err)
	}

	var fleetID string
	err = db.QueryRow(
		ctx,
		`
			SELECT fleet_id
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(&fleetID)
	if err != nil {
		t.Fatalf("resolve fixture fleet: %v", err)
	}

	lifecycleTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fleet lifecycle transaction: %v", err)
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
		"fleet:"+fleetID,
	); err != nil {
		t.Fatalf("acquire fleet lifecycle lock: %v", err)
	}

	assignService := NewService(Dependencies{DB: db})
	assignResult := make(chan error, 1)

	go func() {
		_, err := assignService.Assign(
			context.Background(),
			fixture.UserID,
			AssignDriverRequest{
				VehicleID: fixture.VehicleID,
				Notes:     "fleet lifecycle concurrency integration test",
			},
		)
		assignResult <- err
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
			t.Fatal("assignment never waited on fleet advisory lock")
		}

		time.Sleep(10 * time.Millisecond)
	}

	_, err = lifecycleTx.Exec(
		ctx,
		`
			UPDATE fleets
			SET is_active = FALSE, updated_at = NOW()
			WHERE id = $1
			  AND deleted_at IS NULL
		`,
		fleetID,
	)
	if err != nil {
		t.Fatalf("deactivate fleet while holding lifecycle lock: %v", err)
	}

	if err := lifecycleTx.Commit(ctx); err != nil {
		t.Fatalf("commit fleet deactivation: %v", err)
	}
	committed = true

	select {
	case err := <-assignResult:
		if !errors.Is(err, ErrFleetInactive) {
			t.Fatalf(
				"expected ErrFleetInactive after fleet lifecycle won serialization, got %v",
				err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("assignment did not finish after fleet lifecycle lock released")
	}

	var activeAssignments int
	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_assignments
			WHERE vehicle_id = $1
			  AND unassigned_at IS NULL
		`,
		fixture.VehicleID,
	).Scan(&activeAssignments)
	if err != nil {
		t.Fatalf("count active assignments after fleet race: %v", err)
	}

	if activeAssignments != 0 {
		t.Fatalf(
			"inactive fleet vehicle has %d active assignments; expected 0",
			activeAssignments,
		)
	}
}

func TestAssignRechecksParentLifecycleAfterLock(
	t *testing.T,
) {
	tests := []struct {
		name          string
		lockKey       func(*testutil.DriverFixture) string
		deactivateSQL string
		deactivateID  func(*testutil.DriverFixture) string
		wantErr       error
	}{
		{
			name: "company",
			lockKey: func(f *testutil.DriverFixture) string {
				return "company:" + f.CompanyID
			},
			deactivateSQL: `
				UPDATE companies
				SET is_active = FALSE, updated_at = NOW()
				WHERE id = $1
				  AND deleted_at IS NULL
			`,
			deactivateID: func(f *testutil.DriverFixture) string {
				return f.CompanyID
			},
			wantErr: ErrCompanyInactive,
		},
		{
			name: "branch",
			lockKey: func(f *testutil.DriverFixture) string {
				return "branch:" + f.BranchID
			},
			deactivateSQL: `
				UPDATE branches
				SET is_active = FALSE, updated_at = NOW()
				WHERE id = $1
				  AND deleted_at IS NULL
			`,
			deactivateID: func(f *testutil.DriverFixture) string {
				return f.BranchID
			},
			wantErr: ErrBranchInactive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			t.Cleanup(db.Close)

			fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
			if err != nil {
				t.Fatalf("create driver fixture: %v", err)
			}
			t.Cleanup(func() {
				if err := cleanup(context.Background()); err != nil {
					t.Errorf("cleanup driver fixture: %v", err)
				}
			})

			_, err = db.Exec(
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
				t.Fatalf("close fixture assignment: %v", err)
			}

			_, err = db.Exec(
				ctx,
				`
					INSERT INTO driver_presence (
						driver_id,
						company_id,
						branch_id,
						is_online,
						availability_status
					)
					VALUES ($1, $2, $3, FALSE, 'OFFLINE')
				`,
				fixture.UserID,
				fixture.CompanyID,
				fixture.BranchID,
			)
			if err != nil {
				t.Fatalf("create fixture driver presence: %v", err)
			}

			lifecycleTx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin %s lifecycle transaction: %v", tc.name, err)
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
				tc.lockKey(fixture),
			); err != nil {
				t.Fatalf("acquire %s lifecycle lock: %v", tc.name, err)
			}

			assignService := NewService(Dependencies{DB: db})
			assignResult := make(chan error, 1)

			go func() {
				_, err := assignService.Assign(
					context.Background(),
					fixture.UserID,
					AssignDriverRequest{
						VehicleID: fixture.VehicleID,
						Notes:     tc.name + " lifecycle concurrency integration test",
					},
				)
				assignResult <- err
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
					t.Fatalf(
						"assignment never waited on %s advisory lock",
						tc.name,
					)
				}

				time.Sleep(10 * time.Millisecond)
			}

			_, err = lifecycleTx.Exec(
				ctx,
				tc.deactivateSQL,
				tc.deactivateID(fixture),
			)
			if err != nil {
				t.Fatalf(
					"deactivate %s while holding lifecycle lock: %v",
					tc.name,
					err,
				)
			}

			if err := lifecycleTx.Commit(ctx); err != nil {
				t.Fatalf("commit %s deactivation: %v", tc.name, err)
			}
			committed = true

			select {
			case err := <-assignResult:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf(
						"expected %v after %s lifecycle won serialization, got %v",
						tc.wantErr,
						tc.name,
						err,
					)
				}

			case <-time.After(5 * time.Second):
				t.Fatalf(
					"assignment did not finish after %s lifecycle lock released",
					tc.name,
				)
			}
		})
	}
}

func TestAssignAllowsConcurrentSharedParentLifecycleLocks(
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
	t.Cleanup(db.Close)

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	_, err = db.Exec(
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
		t.Fatalf("close fixture assignment: %v", err)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status
			)
			VALUES ($1, $2, $3, FALSE, 'OFFLINE')
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create fixture driver presence: %v", err)
	}

	var fleetID string
	err = db.QueryRow(
		ctx,
		`
			SELECT fleet_id
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(&fleetID)
	if err != nil {
		t.Fatalf("resolve fixture fleet: %v", err)
	}

	readerTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin parent lifecycle reader transaction: %v", err)
	}

	readerFinished := false
	defer func() {
		if !readerFinished {
			_ = readerTx.Rollback(context.Background())
		}
	}()

	for _, key := range []string{
		"company:" + fixture.CompanyID,
		"branch:" + fixture.BranchID,
		"fleet:" + fleetID,
	} {
		if err := postgresrepo.AcquireTransactionAdvisorySharedLock(
			ctx,
			readerTx,
			key,
		); err != nil {
			t.Fatalf("acquire shared parent lifecycle lock %q: %v", key, err)
		}
	}

	assignCtx, cancelAssign := context.WithTimeout(ctx, 5*time.Second)
	defer cancelAssign()

	assignService := NewService(Dependencies{DB: db})

	assignment, err := assignService.Assign(
		assignCtx,
		fixture.UserID,
		AssignDriverRequest{
			VehicleID: fixture.VehicleID,
			Notes:     "shared parent lifecycle lock compatibility test",
		},
	)
	if err != nil {
		t.Fatalf(
			"assignment should proceed while compatible shared parent locks are held: %v",
			err,
		)
	}
	if assignment == nil {
		t.Fatal("expected created assignment")
	}

	if err := readerTx.Rollback(ctx); err != nil {
		t.Fatalf("release shared parent lifecycle locks: %v", err)
	}
	readerFinished = true

	if _, err := db.Exec(
		ctx,
		`DELETE FROM driver_presence WHERE driver_id = $1`,
		fixture.UserID,
	); err != nil {
		t.Fatalf("delete fixture driver presence: %v", err)
	}

	if _, err := db.Exec(
		ctx,
		`DELETE FROM driver_assignments WHERE id = $1`,
		assignment.ID,
	); err != nil {
		t.Fatalf("delete created assignment: %v", err)
	}
}
