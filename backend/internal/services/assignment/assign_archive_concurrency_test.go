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
