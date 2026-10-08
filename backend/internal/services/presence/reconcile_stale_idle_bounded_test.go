package presence

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestReconcileStaleIdleBoundedSkipsLockedDriver(t *testing.T) {
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)

	cfg.Presence.HeartbeatTimeout = 2 * time.Minute

	staleBefore := now.Add(-cfg.Presence.HeartbeatTimeout)
	staleHeartbeat := staleBefore.Add(-time.Minute)

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
		VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		staleHeartbeat,
	)
	if err != nil {
		t.Fatalf("insert stale driver presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	var driverID string

	err = db.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE user_id = $1`,
		fixture.UserID,
	).Scan(&driverID)
	if err != nil {
		t.Fatalf("get permanent driver ID: %v", err)
	}

	service := &Service{
		db:  db,
		cfg: cfg,
	}

	lockTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin locking transaction: %v", err)
	}

	var lockedDriverID string

	err = lockTx.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE id = $1 FOR UPDATE`,
		driverID,
	).Scan(&lockedDriverID)
	if err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("lock driver aggregate: %v", err)
	}

	// A driver locked by another transaction must not block the worker.
	lockedCtx, cancel := context.WithTimeout(ctx, 3*time.Second)

	_, err = service.ReconcileStaleIdleBounded(
		lockedCtx,
		now,
		10000,
	)

	cancel()

	if err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("reconciliation blocked by driver lock: %v", err)
	}

	var online bool

	err = db.QueryRow(
		ctx,
		`SELECT is_online FROM driver_presence WHERE driver_id = $1`,
		fixture.UserID,
	).Scan(&online)
	if err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("read locked driver presence: %v", err)
	}

	if !online {
		_ = lockTx.Rollback(context.Background())
		t.Fatal("locked driver was incorrectly expired")
	}

	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release driver lock: %v", err)
	}

	// Reconciliation must expire the same driver after its lock is released.
	_, err = service.ReconcileStaleIdleBounded(
		ctx,
		now,
		10000,
	)
	if err != nil {
		t.Fatalf("reconcile unlocked driver: %v", err)
	}

	var status string

	err = db.QueryRow(
		ctx,
		`SELECT is_online, availability_status
		 FROM driver_presence
		 WHERE driver_id = $1`,
		fixture.UserID,
	).Scan(&online, &status)
	if err != nil {
		t.Fatalf("read reconciled presence: %v", err)
	}

	if online || status != "OFFLINE" {
		t.Fatalf(
			"expected unlocked stale driver OFFLINE, got online=%v status=%s",
			online,
			status,
		)
	}
}

// TestReconcileStaleIdleBoundedPreservesRefreshedHeartbeat verifies that
// reconciliation does not expire a driver whose heartbeat has become fresh.
//
// A permanent driver lock prevents reconciliation from proceeding while
// another transaction refreshes the heartbeat. Once that transaction
// commits, reconciliation must preserve the refreshed online presence.
func TestReconcileStaleIdleBoundedPreservesRefreshedHeartbeat(
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
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	cfg.Presence.HeartbeatTimeout = 2 * time.Minute

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-cfg.Presence.HeartbeatTimeout)
	staleHeartbeat := staleBefore.Add(-time.Minute)

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
		VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		staleHeartbeat,
	)
	if err != nil {
		t.Fatalf("insert stale presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	var driverID string
	err = db.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE user_id = $1`,
		fixture.UserID,
	).Scan(&driverID)
	if err != nil {
		t.Fatalf("get permanent driver ID: %v", err)
	}

	service := &Service{
		db:  db,
		cfg: cfg,
	}

	// Hold the permanent driver lock while refreshing its heartbeat.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin heartbeat transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	var lockedID string
	err = tx.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE id = $1 FOR UPDATE`,
		driverID,
	).Scan(&lockedID)
	if err != nil {
		t.Fatalf("lock permanent driver: %v", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE driver_presence
		SET last_heartbeat_at = $2,
		    updated_at = NOW()
		WHERE driver_id = $1
	`,
		fixture.UserID,
		now,
	)
	if err != nil {
		t.Fatalf("refresh heartbeat: %v", err)
	}

	// Reconciliation must skip the locked aggregate.
	lockedCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, err = service.ReconcileStaleIdleBounded(
		lockedCtx,
		now,
		10000,
	)
	cancel()

	if err != nil {
		t.Fatalf("reconcile while heartbeat transaction holds lock: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit refreshed heartbeat: %v", err)
	}

	// After commit, the fresh heartbeat must make this driver ineligible.
	_, err = service.ReconcileStaleIdleBounded(
		ctx,
		now,
		10000,
	)
	if err != nil {
		t.Fatalf("reconcile after heartbeat refresh: %v", err)
	}

	var (
		online    bool
		status    string
		heartbeat time.Time
	)

	err = db.QueryRow(ctx, `
		SELECT is_online, availability_status, last_heartbeat_at
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(
		&online,
		&status,
		&heartbeat,
	)
	if err != nil {
		t.Fatalf("read refreshed presence: %v", err)
	}

	if !online || status != "AVAILABLE" {
		t.Fatalf(
			"refreshed driver incorrectly expired: online=%v status=%s",
			online,
			status,
		)
	}

	if !heartbeat.Equal(now) {
		t.Fatalf(
			"heartbeat changed unexpectedly: got=%s want=%s",
			heartbeat,
			now,
		)
	}
}

// TestReconcileStaleIdleBoundedCursorFairness verifies that a locked
// candidate cannot permanently starve later stale drivers.
//
// With batchSize=1, the first cycle skips the locked driver,
// the second cycle reaches the next driver, and a subsequent cycle
// wraps the cursor to reconsider the first driver.
func TestReconcileStaleIdleBoundedCursorFairness(t *testing.T) {
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

	first, cleanupFirst, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create first driver: %v", err)
	}
	defer func() {
		if err := cleanupFirst(context.Background()); err != nil {
			t.Logf("cleanup first driver: %v", err)
		}
	}()

	second, cleanupSecond, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create second driver: %v", err)
	}
	defer func() {
		if err := cleanupSecond(context.Background()); err != nil {
			t.Logf("cleanup second driver: %v", err)
		}
	}()

	cfg.Presence.HeartbeatTimeout = 2 * time.Minute

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleHeartbeat := now.Add(-10 * time.Minute)

	type candidate struct {
		driverID string
		userID   string
	}

	candidates := []candidate{
		{userID: first.UserID},
		{userID: second.UserID},
	}

	for i := range candidates {
		err = db.QueryRow(
			ctx,
			`SELECT id FROM drivers WHERE user_id = $1`,
			candidates[i].userID,
		).Scan(&candidates[i].driverID)
		if err != nil {
			t.Fatalf("find permanent driver ID: %v", err)
		}
	}

	// Order fixtures by the same permanent driver ID used by the
	// repository's keyset pagination.
	if candidates[0].driverID > candidates[1].driverID {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}

	for _, fixture := range []struct {
		userID       string
		companyID    string
		branchID     string
		vehicleID    string
		assignmentID string
	}{
		{
			first.UserID,
			first.CompanyID,
			first.BranchID,
			first.VehicleID,
			first.AssignmentID,
		},
		{
			second.UserID,
			second.CompanyID,
			second.BranchID,
			second.VehicleID,
			second.AssignmentID,
		},
	} {
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
			VALUES ($1,$2,$3,$4,$5,TRUE,'AVAILABLE',$6)
		`,
			fixture.userID,
			fixture.companyID,
			fixture.branchID,
			fixture.vehicleID,
			fixture.assignmentID,
			staleHeartbeat,
		)
		if err != nil {
			t.Fatalf("insert stale presence: %v", err)
		}
	}

	defer func() {
		for _, candidate := range candidates {
			_, _ = db.Exec(
				context.Background(),
				`DELETE FROM driver_presence WHERE driver_id = $1`,
				candidate.userID,
			)
		}
	}()

	service := &Service{
		db:  db,
		cfg: cfg,
	}

	// Lock the first candidate's permanent aggregate.
	lockTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin driver lock transaction: %v", err)
	}
	defer func() {
		_ = lockTx.Rollback(context.Background())
	}()

	var lockedID string
	err = lockTx.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE id = $1 FOR UPDATE`,
		candidates[0].driverID,
	).Scan(&lockedID)
	if err != nil {
		t.Fatalf("lock first driver: %v", err)
	}

	// Cycle one: the locked first candidate must be skipped.
	count, err := service.ReconcileStaleIdleBounded(ctx, now, 1)
	if err != nil {
		t.Fatalf("first reconciliation: %v", err)
	}
	if count != 0 {
		t.Fatalf("first cycle expired %d drivers, want 0", count)
	}

	// Cycle two: the cursor must advance to the second driver.
	count, err = service.ReconcileStaleIdleBounded(ctx, now, 1)
	if err != nil {
		t.Fatalf("second reconciliation: %v", err)
	}
	if count != 1 {
		t.Fatalf("second cycle expired %d drivers, want 1", count)
	}

	var (
		firstOnline  bool
		secondOnline bool
	)

	err = db.QueryRow(
		ctx,
		`SELECT is_online FROM driver_presence WHERE driver_id = $1`,
		candidates[0].userID,
	).Scan(&firstOnline)
	if err != nil {
		t.Fatalf("read first presence: %v", err)
	}

	err = db.QueryRow(
		ctx,
		`SELECT is_online FROM driver_presence WHERE driver_id = $1`,
		candidates[1].userID,
	).Scan(&secondOnline)
	if err != nil {
		t.Fatalf("read second presence: %v", err)
	}

	if !firstOnline || secondOnline {
		t.Fatalf(
			"unexpected presence: firstOnline=%v secondOnline=%v",
			firstOnline,
			secondOnline,
		)
	}

	// Release the first driver's lock. The next cycle must wrap
	// around and reconcile it.
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release first driver lock: %v", err)
	}

	count, err = service.ReconcileStaleIdleBounded(ctx, now, 1)
	if err != nil {
		t.Fatalf("wraparound reconciliation: %v", err)
	}
	if count != 1 {
		t.Fatalf("wraparound expired %d drivers, want 1", count)
	}

	err = db.QueryRow(
		ctx,
		`SELECT is_online FROM driver_presence WHERE driver_id = $1`,
		candidates[0].userID,
	).Scan(&firstOnline)
	if err != nil {
		t.Fatalf("read first presence after wraparound: %v", err)
	}

	if firstOnline {
		t.Fatal("first driver remained online after cursor wraparound")
	}
}
