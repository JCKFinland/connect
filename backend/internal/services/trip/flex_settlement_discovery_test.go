package trip

import (
	"context"
	"os"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func prepareFlexSettlementDiscoveryFixture(
	t *testing.T,
) (*pgxpool.Pool, string) {
	t.Helper()

	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing settlement integration test on %q",
			databaseName,
		)
	}

	settlement, err :=
		postgresrepo.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	return db, settlement.ID
}

func findFlexSettlementCandidate(
	candidates []postgresrepo.FlexSettlementCandidate,
	settlementID string,
) (
	postgresrepo.FlexSettlementCandidateKind,
	bool,
) {
	for _, candidate := range candidates {
		if candidate.SettlementID == settlementID {
			return candidate.Kind, true
		}
	}

	return "", false
}

func TestFlexSettlementDiscoveryPendingCapture(t *testing.T) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)
	if err != nil {
		t.Fatal(err)
	}

	kind, found := findFlexSettlementCandidate(
		candidates,
		settlementID,
	)

	if !found {
		t.Fatal("new PENDING settlement not discovered")
	}

	if kind != postgresrepo.FlexSettlementCandidateCapture {
		t.Fatalf("expected CAPTURE, got %q", kind)
	}

	var status string
	var attempts int

	if err := db.QueryRow(ctx, `
        SELECT status, attempt_count
        FROM flex_settlements
        WHERE id = $1
    `, settlementID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" || attempts != 0 {
		t.Fatalf(
			"discovery modified settlement: status=%s attempts=%d",
			status,
			attempts,
		)
	}
}

func TestFlexSettlementDiscoveryProcessingRecovery(t *testing.T) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	_, claimed, err :=
		postgresrepo.ClaimFlexSettlementProcessing(
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

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)
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
}

func TestFlexSettlementDiscoveryCaptureRequestedRecovery(t *testing.T) {
	db, settlementID := prepareFlexCaptureMarkerFixture(t)
	ctx := context.Background()

	if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
		ctx,
		db,
		settlementID,
	); err != nil {
		t.Fatal(err)
	}

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)
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
			"capture-requested settlement must be RECOVERY: found=%v kind=%q",
			found,
			kind,
		)
	}
}

func TestFlexSettlementDiscoveryExcludesCancelled(t *testing.T) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	if _, err := db.Exec(ctx, `
        UPDATE flex_settlements
        SET status = 'CANCELLED'
        WHERE id = $1
    `, settlementID); err != nil {
		t.Fatal(err)
	}

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)
	if err != nil {
		t.Fatal(err)
	}

	_, found := findFlexSettlementCandidate(
		candidates,
		settlementID,
	)

	if found {
		t.Fatal("CANCELLED settlement was discovered")
	}
}

func TestFlexSettlementDiscoveryBatchLimit(t *testing.T) {
	db, _ := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			1,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates) > 1 {
		t.Fatalf(
			"batch size exceeded: %d",
			len(candidates),
		)
	}
}

func TestFlexSettlementDiscoveryNilDatabase(t *testing.T) {
	_, err := postgresrepo.DiscoverFlexSettlementCandidates(
		context.Background(),
		nil,
		10,
	)

	if err == nil {
		t.Fatal("expected error for nil database")
	}
}

func TestFlexSettlementDiscoveryExcludesCaptured(t *testing.T) {
	db, settlementID := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	settlement, claimed, err :=
		postgresrepo.ClaimFlexSettlementProcessing(
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

	if _, err := postgresrepo.PrepareFlexSettlementCapture(
		ctx,
		db,
		settlementID,
	); err != nil {
		t.Fatalf(
			"prepare settlement capture: %v",
			err,
		)
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx,
		db,
		settlementID,
		settlement.ProviderAuthorizationID,
	); err != nil {
		t.Fatalf(
			"finalize settlement capture: %v",
			err,
		)
	}

	var status string

	if err := db.QueryRow(
		ctx,
		`SELECT status FROM flex_settlements WHERE id = $1`,
		settlementID,
	).Scan(&status); err != nil {
		t.Fatalf("read settlement status: %v", err)
	}

	if status != "CAPTURED" {
		t.Fatalf(
			"expected CAPTURED settlement, got %s",
			status,
		)
	}

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)

	if err != nil {
		t.Fatalf("discover settlements: %v", err)
	}

	_, found := findFlexSettlementCandidate(
		candidates,
		settlementID,
	)

	if found {
		t.Fatal("CAPTURED settlement was discovered")
	}
}

func TestFlexSettlementDiscoveryDeterministicOrdering(t *testing.T) {
	db, _ := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	first, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)
	if err != nil {
		t.Fatal(err)
	}

	second, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			200,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != len(second) {
		t.Fatalf(
			"candidate count changed: first=%d second=%d",
			len(first),
			len(second),
		)
	}

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf(
				"nondeterministic candidate at index %d: first=%+v second=%+v",
				i,
				first[i],
				second[i],
			)
		}
	}
}

func TestFlexSettlementDiscoveryMaximumBatchLimit(t *testing.T) {
	db, _ := prepareFlexSettlementDiscoveryFixture(t)
	ctx := context.Background()

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			1000000,
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates) >
		postgresrepo.MaxFlexSettlementDiscoveryBatchSize {
		t.Fatalf(
			"maximum batch exceeded: got=%d maximum=%d",
			len(candidates),
			postgresrepo.MaxFlexSettlementDiscoveryBatchSize,
		)
	}
}

func TestFlexSettlementDiscoveryFairCategoryAllocation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	ctx := context.Background()

	const (
		perCategory = 6
		batchSize   = 6
	)

	type fixtureRecord struct {
		id     string
		status string
	}

	records := make([]fixtureRecord, 0, perCategory*2)

	var db *pgxpool.Pool

	for i := 0; i < perCategory*2; i++ {
		fixtureDB, settlementID :=
			prepareFlexSettlementDiscoveryFixture(t)

		if db == nil {
			db = fixtureDB
		}

		status := "PENDING"

		if i >= perCategory {
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

			status = "PROCESSING"
		}

		records = append(
			records,
			fixtureRecord{
				id:     settlementID,
				status: status,
			},
		)
	}

	// Make only this test's fixtures older than the existing
	// isolated-test backlog. Restore timestamps at cleanup.
	originalTimes := make(map[string]string, len(records))

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		for id, original := range originalTimes {
			if _, err := db.Exec(
				cleanupCtx,
				`
                UPDATE flex_settlements
                SET created_at = $2::timestamptz
                WHERE id = $1
                `,
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

	for i, record := range records {
		var original string

		if err := db.QueryRow(
			ctx,
			`
            SELECT created_at::text
            FROM flex_settlements
            WHERE id = $1
            `,
			record.id,
		).Scan(&original); err != nil {
			t.Fatal(err)
		}

		originalTimes[record.id] = original

		_, err := db.Exec(
			ctx,
			`
            UPDATE flex_settlements
            SET created_at =
                TIMESTAMPTZ '2000-01-01 00:00:00+00'
                + ($2::integer * INTERVAL '1 second')
            WHERE id = $1
            `,
			record.id,
			i,
		)

		if err != nil {
			t.Fatalf(
				"order fairness fixture %s: %v",
				record.id,
				err,
			)
		}
	}

	candidates, err :=
		postgresrepo.DiscoverFlexSettlementCandidates(
			ctx,
			db,
			batchSize,
		)

	if err != nil {
		t.Fatalf("discover fair settlement batch: %v", err)
	}

	if len(candidates) != batchSize {
		t.Fatalf(
			"expected %d candidates, got %d",
			batchSize,
			len(candidates),
		)
	}

	fixtureIDs := make(map[string]string, len(records))

	for _, record := range records {
		fixtureIDs[record.id] = record.status
	}

	captureCount := 0
	recoveryCount := 0

	for _, candidate := range candidates {
		expectedStatus, exists := fixtureIDs[candidate.SettlementID]

		if !exists {
			t.Fatalf(
				"unexpected settlement in fairness batch: %s",
				candidate.SettlementID,
			)
		}

		switch candidate.Kind {
		case postgresrepo.FlexSettlementCandidateCapture:
			if expectedStatus != "PENDING" {
				t.Fatalf(
					"CAPTURE candidate has status %s",
					expectedStatus,
				)
			}

			captureCount++

		case postgresrepo.FlexSettlementCandidateRecovery:
			if expectedStatus != "PROCESSING" {
				t.Fatalf(
					"RECOVERY candidate has status %s",
					expectedStatus,
				)
			}

			recoveryCount++

		default:
			t.Fatalf(
				"unexpected candidate kind %q",
				candidate.Kind,
			)
		}
	}

	if captureCount != 3 || recoveryCount != 3 {
		t.Fatalf(
			"unfair allocation: capture=%d recovery=%d",
			captureCount,
			recoveryCount,
		)
	}

	for _, record := range records {
		var status string
		var attempts int

		if err := db.QueryRow(
			ctx,
			`
            SELECT status, attempt_count
            FROM flex_settlements
            WHERE id = $1
            `,
			record.id,
		).Scan(&status, &attempts); err != nil {
			t.Fatal(err)
		}

		expectedAttempts := 0

		if record.status == "PROCESSING" {
			expectedAttempts = 1
		}

		if status != record.status ||
			attempts != expectedAttempts {
			t.Fatalf(
				"discovery mutated settlement %s: status=%s attempts=%d",
				record.id,
				status,
				attempts,
			)
		}
	}
}

func TestFlexSettlementCategoryCursorPagination(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	ctx := context.Background()

	type record struct {
		id   string
		kind postgresrepo.FlexSettlementCandidateKind
	}

	records := make([]record, 0, 8)

	var db *pgxpool.Pool

	for i := 0; i < 8; i++ {
		fixtureDB, settlementID :=
			prepareFlexSettlementDiscoveryFixture(t)

		if db == nil {
			db = fixtureDB
		}

		kind := postgresrepo.FlexSettlementCandidateCapture

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
		}

		records = append(
			records,
			record{
				id:   settlementID,
				kind: kind,
			},
		)
	}

	originalTimes := make(map[string]string, len(records))

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		for id, original := range originalTimes {
			if _, err := db.Exec(
				cleanupCtx,
				`
                UPDATE flex_settlements
                SET created_at = $2::timestamptz
                WHERE id = $1
                `,
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

	for i, item := range records {
		var original string

		if err := db.QueryRow(
			ctx,
			`
            SELECT created_at::text
            FROM flex_settlements
            WHERE id = $1
            `,
			item.id,
		).Scan(&original); err != nil {
			t.Fatal(err)
		}

		originalTimes[item.id] = original

		_, err := db.Exec(
			ctx,
			`
            UPDATE flex_settlements
            SET created_at =
                TIMESTAMPTZ '1999-01-01 00:00:00+00'
                + ($2::integer * INTERVAL '1 second')
            WHERE id = $1
            `,
			item.id,
			i,
		)

		if err != nil {
			t.Fatal(err)
		}
	}

	captureCursor := ""
	recoveryCursor := ""

	seenCapture := make(map[string]bool)
	seenRecovery := make(map[string]bool)

	for cycle := 0; cycle < 2; cycle++ {
		page, err :=
			postgresrepo.DiscoverFlexSettlementCategoryPage(
				ctx,
				db,
				captureCursor,
				recoveryCursor,
				4,
			)

		if err != nil {
			t.Fatalf(
				"discover category page %d: %v",
				cycle,
				err,
			)
		}

		if len(page.Capture) != 2 ||
			len(page.Recovery) != 2 {
			t.Fatalf(
				"unexpected page sizes: capture=%d recovery=%d",
				len(page.Capture),
				len(page.Recovery),
			)
		}

		for _, candidate := range page.Capture {
			if candidate.Kind !=
				postgresrepo.FlexSettlementCandidateCapture {
				t.Fatalf(
					"wrong capture kind: %s",
					candidate.Kind,
				)
			}

			if seenCapture[candidate.SettlementID] {
				t.Fatalf(
					"duplicate capture candidate %s",
					candidate.SettlementID,
				)
			}

			seenCapture[candidate.SettlementID] = true
		}

		for _, candidate := range page.Recovery {
			if candidate.Kind !=
				postgresrepo.FlexSettlementCandidateRecovery {
				t.Fatalf(
					"wrong recovery kind: %s",
					candidate.Kind,
				)
			}

			if seenRecovery[candidate.SettlementID] {
				t.Fatalf(
					"duplicate recovery candidate %s",
					candidate.SettlementID,
				)
			}

			seenRecovery[candidate.SettlementID] = true
		}

		captureCursor =
			page.Capture[len(page.Capture)-1].SettlementID

		recoveryCursor =
			page.Recovery[len(page.Recovery)-1].SettlementID
	}

	for _, item := range records {
		switch item.kind {
		case postgresrepo.FlexSettlementCandidateCapture:
			if !seenCapture[item.id] {
				t.Fatalf(
					"capture settlement skipped: %s",
					item.id,
				)
			}

		case postgresrepo.FlexSettlementCandidateRecovery:
			if !seenRecovery[item.id] {
				t.Fatalf(
					"recovery settlement skipped: %s",
					item.id,
				)
			}
		}
	}

	if len(seenCapture) != 4 || len(seenRecovery) != 4 {
		t.Fatalf(
			"unexpected unique counts: capture=%d recovery=%d",
			len(seenCapture),
			len(seenRecovery),
		)
	}

	// End-of-category is checked using the last fixture cursor.
	// Additional records created by other tests may exist later.
	page, err :=
		postgresrepo.DiscoverFlexSettlementCategoryPage(
			ctx,
			db,
			captureCursor,
			recoveryCursor,
			4,
		)

	if err != nil {
		t.Fatal(err)
	}

	for _, candidate := range page.Capture {
		if seenCapture[candidate.SettlementID] {
			t.Fatalf(
				"capture cursor returned earlier candidate %s",
				candidate.SettlementID,
			)
		}
	}

	for _, candidate := range page.Recovery {
		if seenRecovery[candidate.SettlementID] {
			t.Fatalf(
				"recovery cursor returned earlier candidate %s",
				candidate.SettlementID,
			)
		}
	}
}

func TestFlexSettlementCategoryCapacityRedistribution(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	ctx := context.Background()

	type fixture struct {
		id   string
		kind postgresrepo.FlexSettlementCandidateKind
	}

	fixtures := make([]fixture, 0, 12)

	var db *pgxpool.Pool

	for i := 0; i < 12; i++ {
		fixtureDB, settlementID :=
			prepareFlexSettlementDiscoveryFixture(t)

		if db == nil {
			db = fixtureDB
		}

		kind := postgresrepo.FlexSettlementCandidateCapture

		if i >= 6 {
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
		}

		fixtures = append(
			fixtures,
			fixture{
				id:   settlementID,
				kind: kind,
			},
		)
	}

	originalTimes := make(map[string]string, len(fixtures))

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		for id, original := range originalTimes {
			if _, err := db.Exec(
				cleanupCtx,
				`
                UPDATE flex_settlements
                SET created_at = $2::timestamptz
                WHERE id = $1
                `,
				id,
				original,
			); err != nil {
				t.Errorf(
					"restore redistribution timestamp %s: %v",
					id,
					err,
				)
			}
		}
	})

	// Place test fixtures ahead of unrelated settlement records.
	for i, item := range fixtures {
		var original string

		if err := db.QueryRow(
			ctx,
			`
            SELECT created_at::text
            FROM flex_settlements
            WHERE id = $1
            `,
			item.id,
		).Scan(&original); err != nil {
			t.Fatal(err)
		}

		originalTimes[item.id] = original

		_, err := db.Exec(
			ctx,
			`
            UPDATE flex_settlements
            SET created_at =
                TIMESTAMPTZ '1998-01-01 00:00:00+00'
                + ($2::integer * INTERVAL '1 second')
            WHERE id = $1
            `,
			item.id,
			i,
		)

		if err != nil {
			t.Fatal(err)
		}
	}

	captureIDs := make([]string, 0, 6)
	recoveryIDs := make([]string, 0, 6)

	for _, item := range fixtures {
		if item.kind == postgresrepo.FlexSettlementCandidateCapture {
			captureIDs = append(captureIDs, item.id)
		} else {
			recoveryIDs = append(recoveryIDs, item.id)
		}
	}

	tests := []struct {
		name           string
		captureCursor  string
		recoveryCursor string
		batchSize      int
		wantCapture    int
		wantRecovery   int
	}{
		{
			name:         "balanced",
			batchSize:    4,
			wantCapture:  2,
			wantRecovery: 2,
		},
		{
			name:           "capture only",
			recoveryCursor: recoveryIDs[len(recoveryIDs)-1],
			batchSize:      4,
			wantCapture:    4,
			wantRecovery:   0,
		},
		{
			name:          "recovery only",
			captureCursor: captureIDs[len(captureIDs)-1],
			batchSize:     4,
			wantCapture:   0,
			wantRecovery:  4,
		},
		{
			name:          "uneven",
			captureCursor: captureIDs[3],
			batchSize:     4,
			wantCapture:   2,
			wantRecovery:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err :=
				postgresrepo.DiscoverFlexSettlementCategoryPage(
					ctx,
					db,
					tc.captureCursor,
					tc.recoveryCursor,
					tc.batchSize,
				)

			if err != nil {
				t.Fatal(err)
			}

			if len(page.Capture) != tc.wantCapture ||
				len(page.Recovery) != tc.wantRecovery {
				t.Fatalf(
					"allocation mismatch: capture=%d recovery=%d; want capture=%d recovery=%d",
					len(page.Capture),
					len(page.Recovery),
					tc.wantCapture,
					tc.wantRecovery,
				)
			}

			if len(page.Capture)+len(page.Recovery) >
				tc.batchSize {
				t.Fatalf(
					"batch limit exceeded: %d > %d",
					len(page.Capture)+len(page.Recovery),
					tc.batchSize,
				)
			}

			seen := make(map[string]bool)

			for _, candidate := range page.Capture {
				if candidate.Kind !=
					postgresrepo.FlexSettlementCandidateCapture {
					t.Fatalf(
						"wrong capture classification: %s",
						candidate.Kind,
					)
				}

				if seen[candidate.SettlementID] {
					t.Fatalf(
						"duplicate settlement: %s",
						candidate.SettlementID,
					)
				}

				seen[candidate.SettlementID] = true
			}

			for _, candidate := range page.Recovery {
				if candidate.Kind !=
					postgresrepo.FlexSettlementCandidateRecovery {
					t.Fatalf(
						"wrong recovery classification: %s",
						candidate.Kind,
					)
				}

				if seen[candidate.SettlementID] {
					t.Fatalf(
						"duplicate settlement: %s",
						candidate.SettlementID,
					)
				}

				seen[candidate.SettlementID] = true
			}
		})
	}
}
