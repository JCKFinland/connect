package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestDriverPresenceRepositoryListStaleIdleCandidates(t *testing.T) {
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
	t.Cleanup(db.Close)

	repo := &DriverPresenceRepository{db: db}

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-2 * time.Minute)
	staleHeartbeat := staleBefore.Add(-time.Second)
	freshHeartbeat := staleBefore.Add(time.Second)
	exactHeartbeat := staleBefore

	type testCase struct {
		name      string
		status    string
		online    bool
		heartbeat *time.Time
		expected  bool
	}

	cases := []testCase{
		{"stale available", "AVAILABLE", true, &staleHeartbeat, true},
		{"stale break", "BREAK", true, &staleHeartbeat, true},
		{"null heartbeat", "AVAILABLE", true, nil, true},
		{"fresh available", "AVAILABLE", true, &freshHeartbeat, false},
		{"exact cutoff", "AVAILABLE", true, &exactHeartbeat, false},
		{"busy driver", "BUSY", true, &staleHeartbeat, false},
		{"offline driver", "OFFLINE", false, &staleHeartbeat, false},
	}

	expected := make(map[string]string)

	for _, tc := range cases {
		fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
		if err != nil {
			t.Fatalf("create fixture for %s: %v", tc.name, err)
		}

		t.Cleanup(func() {
			if err := cleanup(context.Background()); err != nil {
				t.Logf("cleanup fixture: %v", err)
			}
		})

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
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`,
			fixture.UserID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.VehicleID,
			fixture.AssignmentID,
			tc.online,
			tc.status,
			tc.heartbeat,
		)
		if err != nil {
			t.Fatalf("insert presence for %s: %v", tc.name, err)
		}

		userID := fixture.UserID
		t.Cleanup(func() {
			_, _ = db.Exec(
				context.Background(),
				`DELETE FROM driver_presence WHERE driver_id = $1`,
				userID,
			)
		})

		if tc.expected {
			var permanentDriverID string
			err = db.QueryRow(ctx,
				`SELECT id FROM drivers WHERE user_id = $1`,
				fixture.UserID,
			).Scan(&permanentDriverID)
			if err != nil {
				t.Fatalf("get permanent driver ID: %v", err)
			}

			expected[permanentDriverID] = fixture.UserID
		}
	}

	// Pagination must progress beyond the first page without duplicates.
	// Other database fixtures may also qualify, so assert membership rather
	// than assuming this test owns every eligible presence row.
	found := make(map[string]string)
	cursor := ""
	const pageSize = 2

	for page := 0; page < 10000; page++ {
		candidates, err := repo.ListStaleIdleCandidates(
			ctx,
			staleBefore,
			cursor,
			pageSize,
		)
		if err != nil {
			t.Fatalf("list stale candidates: %v", err)
		}
		if len(candidates) > pageSize {
			t.Fatalf("page exceeds limit: %d", len(candidates))
		}
		if len(candidates) == 0 {
			break
		}

		for _, candidate := range candidates {
			if _, duplicate := found[candidate.DriverID]; duplicate {
				t.Fatalf("duplicate driver ID: %s", candidate.DriverID)
			}

			if cursor != "" {
				previous, err := uuid.Parse(cursor)
				if err != nil {
					t.Fatalf("parse previous cursor: %v", err)
				}
				current, err := uuid.Parse(candidate.DriverID)
				if err != nil {
					t.Fatalf("parse candidate ID: %v", err)
				}
				if previous.String() >= current.String() {
					t.Fatalf("pagination did not advance")
				}
			}

			found[candidate.DriverID] = candidate.UserID
			cursor = candidate.DriverID
		}

		if len(candidates) < pageSize {
			break
		}

		if page == 9999 {
			t.Fatal("pagination exceeded safety limit")
		}
	}

	for driverID, userID := range expected {
		if found[driverID] != userID {
			t.Errorf(
				"expected driver %s mapped to user %s, got %s",
				driverID,
				userID,
				found[driverID],
			)
		}
	}
}

func TestDriverPresenceRepositoryListStaleIdleCandidatesValidation(t *testing.T) {
	repo := &DriverPresenceRepository{}

	_, err := repo.ListStaleIdleCandidates(
		context.Background(),
		time.Time{},
		"",
		10,
	)
	if err == nil {
		t.Fatal("expected zero cutoff validation error")
	}

	_, err = repo.ListStaleIdleCandidates(
		context.Background(),
		time.Now().UTC(),
		"",
		0,
	)
	if err == nil {
		t.Fatal("expected invalid limit validation error")
	}
}

func TestDriverPresenceRepositoryListStaleIdleCandidatesDoesNotLockDriver(
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
			t.Logf("cleanup fixture: %v", err)
		}
	}()

	staleBefore := time.Now().UTC().Add(-2 * time.Minute)

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
		staleBefore.Add(-time.Second),
	)
	if err != nil {
		t.Fatalf("insert presence: %v", err)
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

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin driver lock transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var lockedID string
	err = tx.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE id = $1 FOR UPDATE`,
		driverID,
	).Scan(&lockedID)
	if err != nil {
		t.Fatalf("lock permanent driver: %v", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	repo := &DriverPresenceRepository{db: db}
	candidates, err := repo.ListStaleIdleCandidates(
		queryCtx,
		staleBefore,
		"",
		10000,
	)
	if err != nil {
		t.Fatalf("candidate discovery blocked by driver lock: %v", err)
	}

	for _, candidate := range candidates {
		if candidate.DriverID == driverID {
			if candidate.UserID != fixture.UserID {
				t.Fatalf("incorrect user ID mapping")
			}
			return
		}
	}

	t.Fatal("locked driver was not returned by read-only discovery")
}
