package trip

import (
	"context"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFlexSettlementSchedulerPostgresCaptureDiscovery(
	t *testing.T,
) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 200)
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}

	kind, found := findFlexSettlementCandidate(
		candidates,
		settlementID,
	)

	if !found {
		t.Fatal("scheduler did not discover PENDING settlement")
	}

	if kind != postgresrepo.FlexSettlementCandidateCapture {
		t.Fatalf("category = %q, want CAPTURE", kind)
	}

	var status string
	var attempts int

	err = db.QueryRow(ctx, `
        SELECT status, attempt_count
        FROM flex_settlements
        WHERE id = $1
    `, settlementID).Scan(&status, &attempts)

	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" || attempts != 0 {
		t.Fatalf(
			"scheduler modified settlement: status=%s attempts=%d",
			status,
			attempts,
		)
	}
}

func TestFlexSettlementSchedulerPostgresRecoveryDiscovery(
	t *testing.T,
) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx,
		db,
		settlementID,
	)

	if err != nil || !claimed {
		t.Fatalf(
			"claim fixture settlement: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 200)
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}

	kind, found := findFlexSettlementCandidate(
		candidates,
		settlementID,
	)

	if !found ||
		kind != postgresrepo.FlexSettlementCandidateRecovery {
		t.Fatalf(
			"expected RECOVERY: found=%v kind=%q",
			found,
			kind,
		)
	}

	var status string

	if err := db.QueryRow(ctx, `
        SELECT status
        FROM flex_settlements
        WHERE id = $1
    `, settlementID).Scan(&status); err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" {
		t.Fatalf(
			"scheduler changed settlement status to %s",
			status,
		)
	}
}

func TestFlexSettlementSchedulerPostgresBatchOneFairness(
	t *testing.T,
) {
	db, captureID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	_, recoveryID := prepareFlexSettlementDiscoveryFixture(t)

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx,
		db,
		recoveryID,
	)

	if err != nil || !claimed {
		t.Fatalf(
			"claim recovery fixture: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 1)
	if err != nil {
		t.Fatal(err)
	}

	foundCapture := false
	foundRecovery := false

	for i := 0; i < 20; i++ {
		candidates, err := scheduler.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if len(candidates) > 1 {
			t.Fatalf(
				"batch size one returned %d candidates",
				len(candidates),
			)
		}

		for _, candidate := range candidates {
			if candidate.SettlementID == captureID &&
				candidate.Kind == postgresrepo.FlexSettlementCandidateCapture {
				foundCapture = true
			}

			if candidate.SettlementID == recoveryID &&
				candidate.Kind == postgresrepo.FlexSettlementCandidateRecovery {
				foundRecovery = true
			}
		}

		if foundCapture && foundRecovery {
			return
		}
	}

	t.Fatalf(
		"scheduler fairness failed: capture=%v recovery=%v",
		foundCapture,
		foundRecovery,
	)
}

func TestFlexSettlementSchedulerPostgresExhaustivePagination(
	t *testing.T,
) {
	ctx := context.Background()

	type fixture struct {
		id     string
		kind   postgresrepo.FlexSettlementCandidateKind
		status string
	}

	fixtures := make([]fixture, 0, 8)
	var db *pgxpool.Pool

	for i := 0; i < 8; i++ {
		fixtureDB, settlementID :=
			prepareFlexSettlementDiscoveryFixture(t)

		if db == nil {
			db = fixtureDB
		}

		kind := postgresrepo.FlexSettlementCandidateCapture
		status := "PENDING"

		if i%2 == 1 {
			_, claimed, err :=
				postgresrepo.ClaimFlexSettlementProcessing(
					ctx,
					db,
					settlementID,
				)

			if err != nil || !claimed {
				t.Fatalf(
					"claim recovery fixture: claimed=%v err=%v",
					claimed,
					err,
				)
			}

			kind = postgresrepo.FlexSettlementCandidateRecovery
			status = "PROCESSING"
		}

		fixtures = append(fixtures, fixture{
			id:     settlementID,
			kind:   kind,
			status: status,
		})
	}

	originalTimes := make(map[string]string, len(fixtures))

	t.Cleanup(func() {
		for id, original := range originalTimes {
			if _, err := db.Exec(
				context.Background(),
				`UPDATE flex_settlements
                 SET created_at = $2::timestamptz
                 WHERE id = $1`,
				id,
				original,
			); err != nil {
				t.Errorf(
					"restore settlement timestamp %s: %v",
					id,
					err,
				)
			}
		}
	})

	// Place fixtures ahead of unrelated records.
	for i, item := range fixtures {
		var original string

		if err := db.QueryRow(
			ctx,
			`SELECT created_at::text
             FROM flex_settlements
             WHERE id = $1`,
			item.id,
		).Scan(&original); err != nil {
			t.Fatal(err)
		}

		originalTimes[item.id] = original

		if _, err := db.Exec(
			ctx,
			`UPDATE flex_settlements
             SET created_at =
                 TIMESTAMPTZ '1997-01-01 00:00:00+00'
                 + ($2::integer * INTERVAL '1 second')
             WHERE id = $1`,
			item.id,
			i,
		); err != nil {
			t.Fatal(err)
		}
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 4)
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	expected := make(map[string]fixture)

	for _, item := range fixtures {
		expected[item.id] = item
	}

	// Two batches must visit all eight historical fixtures.
	for cycle := 0; cycle < 2; cycle++ {
		candidates, err := scheduler.Next(ctx)
		if err != nil {
			t.Fatalf("cycle %d: %v", cycle, err)
		}

		if len(candidates) != 4 {
			t.Fatalf(
				"cycle %d: got %d candidates, want 4",
				cycle,
				len(candidates),
			)
		}

		batchSeen := make(map[string]bool)

		for _, candidate := range candidates {
			item, exists := expected[candidate.SettlementID]
			if !exists {
				t.Fatalf(
					"unexpected candidate before fixture exhaustion: %s",
					candidate.SettlementID,
				)
			}

			if batchSeen[candidate.SettlementID] {
				t.Fatalf(
					"duplicate in cycle %d: %s",
					cycle,
					candidate.SettlementID,
				)
			}

			if seen[candidate.SettlementID] {
				t.Fatalf(
					"fixture repeated before full traversal: %s",
					candidate.SettlementID,
				)
			}

			if candidate.Kind != item.kind {
				t.Fatalf(
					"incorrect category for %s: got %s want %s",
					candidate.SettlementID,
					candidate.Kind,
					item.kind,
				)
			}

			batchSeen[candidate.SettlementID] = true
			seen[candidate.SettlementID] = true
		}
	}

	if len(seen) != len(fixtures) {
		t.Fatalf(
			"visited %d of %d fixtures",
			len(seen),
			len(fixtures),
		)
	}

	// The next call must wrap both exhausted categories.
	wrapped, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(wrapped) != 4 {
		t.Fatalf(
			"wraparound returned %d candidates, want 4",
			len(wrapped),
		)
	}

	expectedWrap := map[string]bool{
		fixtures[0].id: true,
		fixtures[1].id: true,
		fixtures[2].id: true,
		fixtures[3].id: true,
	}

	for _, candidate := range wrapped {
		if !expectedWrap[candidate.SettlementID] {
			t.Fatalf(
				"unexpected wraparound candidate %s",
				candidate.SettlementID,
			)
		}
	}

	// Discovery must not alter settlement state or attempt count.
	for _, item := range fixtures {
		var status string
		var attempts int

		if err := db.QueryRow(
			ctx,
			`SELECT status, attempt_count
             FROM flex_settlements
             WHERE id = $1`,
			item.id,
		).Scan(&status, &attempts); err != nil {
			t.Fatal(err)
		}

		expectedAttempts := 0
		if item.kind == postgresrepo.FlexSettlementCandidateRecovery {
			expectedAttempts = 1
		}

		if status != item.status || attempts != expectedAttempts {
			t.Fatalf(
				"discovery changed settlement %s: status=%s attempts=%d",
				item.id,
				status,
				attempts,
			)
		}
	}
}

func TestFlexSettlementSchedulerPostgresCategoryTransition(
	t *testing.T,
) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 200)
	if err != nil {
		t.Fatal(err)
	}

	initial, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}

	kind, found := findFlexSettlementCandidate(
		initial,
		settlementID,
	)

	if !found ||
		kind != postgresrepo.FlexSettlementCandidateCapture {
		t.Fatalf(
			"expected initial CAPTURE: found=%v kind=%q",
			found,
			kind,
		)
	}

	_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
		ctx,
		db,
		settlementID,
	)

	if err != nil || !claimed {
		t.Fatalf(
			"claim settlement: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	foundRecovery := false

	for cycle := 0; cycle < 10; cycle++ {
		candidates, err := scheduler.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}

		occurrences := 0

		for _, candidate := range candidates {
			if candidate.SettlementID != settlementID {
				continue
			}

			occurrences++

			if candidate.Kind !=
				postgresrepo.FlexSettlementCandidateRecovery {
				t.Fatalf(
					"transitioned settlement returned as %s",
					candidate.Kind,
				)
			}

			foundRecovery = true
		}

		if occurrences > 1 {
			t.Fatalf(
				"transitioned settlement duplicated in batch %d",
				cycle,
			)
		}

		if foundRecovery {
			break
		}
	}

	if !foundRecovery {
		t.Fatal(
			"transitioned settlement was not discovered as RECOVERY",
		)
	}

	var status string
	var attempts int

	if err := db.QueryRow(
		ctx,
		`SELECT status, attempt_count
         FROM flex_settlements
         WHERE id = $1`,
		settlementID,
	).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || attempts != 1 {
		t.Fatalf(
			"discovery changed transition state: status=%s attempts=%d",
			status,
			attempts,
		)
	}
}

func TestFlexSettlementSchedulerPostgresCandidateRemoval(
	t *testing.T,
) {
	ctx := context.Background()

	db, removedID := prepareFlexSettlementDiscoveryFixture(t)
	_, remainingID := prepareFlexSettlementDiscoveryFixture(t)

	originalTimes := make(map[string]string, 2)

	t.Cleanup(func() {
		for id, original := range originalTimes {
			if _, err := db.Exec(
				context.Background(),
				`UPDATE flex_settlements
                 SET created_at = $2::timestamptz
                 WHERE id = $1`,
				id,
				original,
			); err != nil {
				t.Errorf("restore timestamp %s: %v", id, err)
			}
		}
	})

	for i, id := range []string{removedID, remainingID} {
		var original string

		if err := db.QueryRow(
			ctx,
			`SELECT created_at::text
             FROM flex_settlements
             WHERE id = $1`,
			id,
		).Scan(&original); err != nil {
			t.Fatal(err)
		}

		originalTimes[id] = original

		if _, err := db.Exec(
			ctx,
			`UPDATE flex_settlements
             SET created_at =
                 TIMESTAMPTZ '1996-01-01 00:00:00+00'
                 + ($2::integer * INTERVAL '1 second')
             WHERE id = $1`,
			id,
			i,
		); err != nil {
			t.Fatal(err)
		}
	}

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 1)
	if err != nil {
		t.Fatal(err)
	}

	initial, err := scheduler.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(initial) != 1 ||
		initial[0].SettlementID != removedID {
		t.Fatalf(
			"expected oldest settlement first, got %+v",
			initial,
		)
	}

	tag, err := db.Exec(
		ctx,
		`UPDATE flex_settlements
         SET status = 'CANCELLED'
         WHERE id = $1 AND status = 'PENDING'`,
		removedID,
	)

	if err != nil {
		t.Fatal(err)
	}

	if tag.RowsAffected() != 1 {
		t.Fatalf(
			"expected one cancelled fixture, got %d",
			tag.RowsAffected(),
		)
	}

	foundRemaining := false

	for cycle := 0; cycle < 10; cycle++ {
		candidates, err := scheduler.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if len(candidates) > 1 {
			t.Fatalf(
				"batch size exceeded in cycle %d",
				cycle,
			)
		}

		for _, candidate := range candidates {
			if candidate.SettlementID == removedID {
				t.Fatal(
					"cancelled settlement rediscovered",
				)
			}

			if candidate.SettlementID == remainingID {
				if candidate.Kind !=
					postgresrepo.FlexSettlementCandidateCapture {
					t.Fatalf(
						"remaining candidate classified as %s",
						candidate.Kind,
					)
				}

				foundRemaining = true
			}
		}

		if foundRemaining {
			break
		}
	}

	if !foundRemaining {
		t.Fatal(
			"remaining eligible settlement was skipped",
		)
	}

	var cancelledStatus string
	var cancelledAttempts int

	if err := db.QueryRow(
		ctx,
		`SELECT status, attempt_count
         FROM flex_settlements
         WHERE id = $1`,
		removedID,
	).Scan(
		&cancelledStatus,
		&cancelledAttempts,
	); err != nil {
		t.Fatal(err)
	}

	if cancelledStatus != "CANCELLED" ||
		cancelledAttempts != 0 {
		t.Fatalf(
			"cancelled settlement changed: status=%s attempts=%d",
			cancelledStatus,
			cancelledAttempts,
		)
	}

	var remainingStatus string
	var remainingAttempts int

	if err := db.QueryRow(
		ctx,
		`SELECT status, attempt_count
         FROM flex_settlements
         WHERE id = $1`,
		remainingID,
	).Scan(
		&remainingStatus,
		&remainingAttempts,
	); err != nil {
		t.Fatal(err)
	}

	if remainingStatus != "PENDING" ||
		remainingAttempts != 0 {
		t.Fatalf(
			"scheduler modified remaining settlement: status=%s attempts=%d",
			remainingStatus,
			remainingAttempts,
		)
	}
}

func TestFlexSettlementSchedulerPostgresNoDuplicateBatch(
	t *testing.T,
) {
	db, _ := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	scheduler, err := flexsettlement.NewDiscoveryScheduler(db, 4)
	if err != nil {
		t.Fatal(err)
	}

	for iteration := 0; iteration < 10; iteration++ {
		candidates, err := scheduler.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if len(candidates) > 4 {
			t.Fatalf(
				"iteration %d exceeded batch limit",
				iteration,
			)
		}

		seen := make(map[string]bool)

		for _, candidate := range candidates {
			if seen[candidate.SettlementID] {
				t.Fatalf(
					"iteration %d duplicated settlement %s",
					iteration,
					candidate.SettlementID,
				)
			}

			seen[candidate.SettlementID] = true
		}
	}
}
