package flex_settlement

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type discoveryQueryCall struct {
	kind    postgresrepo.FlexSettlementCandidateKind
	afterID string
	limit   int
}

func newMockDiscoveryScheduler(
	batchSize int,
	captureIDs []string,
	recoveryIDs []string,
) (*DiscoveryScheduler, *[]discoveryQueryCall) {
	calls := make([]discoveryQueryCall, 0)

	scheduler := &DiscoveryScheduler{
		db:        &pgxpool.Pool{},
		batchSize: batchSize,
	}

	scheduler.query = func(
		ctx context.Context,
		db *pgxpool.Pool,
		kind postgresrepo.FlexSettlementCandidateKind,
		afterID string,
		limit int,
	) ([]postgresrepo.FlexSettlementCandidate, error) {
		calls = append(calls, discoveryQueryCall{
			kind:    kind,
			afterID: afterID,
			limit:   limit,
		})

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var ids []string

		switch kind {
		case postgresrepo.FlexSettlementCandidateCapture:
			ids = captureIDs
		case postgresrepo.FlexSettlementCandidateRecovery:
			ids = recoveryIDs
		default:
			return nil, fmt.Errorf("unexpected category: %s", kind)
		}

		start := 0

		if afterID != "" {
			start = len(ids)

			for i, id := range ids {
				if id == afterID {
					start = i + 1
					break
				}
			}
		}

		if start >= len(ids) {
			return nil, nil
		}

		end := start + limit

		if end > len(ids) {
			end = len(ids)
		}

		result := make(
			[]postgresrepo.FlexSettlementCandidate,
			0,
			end-start,
		)

		for _, id := range ids[start:end] {
			result = append(
				result,
				postgresrepo.FlexSettlementCandidate{
					SettlementID: id,
					Kind:         kind,
				},
			)
		}

		return result, nil
	}

	return scheduler, &calls
}

func assertDiscoveryIDs(
	t *testing.T,
	candidates []postgresrepo.FlexSettlementCandidate,
	expected ...string,
) {
	t.Helper()

	actual := make([]string, 0, len(candidates))
	seen := make(map[string]bool)

	for _, candidate := range candidates {
		if seen[candidate.SettlementID] {
			t.Fatalf(
				"duplicate settlement returned: %s",
				candidate.SettlementID,
			)
		}

		seen[candidate.SettlementID] = true
		actual = append(actual, candidate.SettlementID)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"candidate IDs = %v, want %v",
			actual,
			expected,
		)
	}
}

func TestDiscoverySchedulerBatchOneAlternatesCategories(
	t *testing.T,
) {
	scheduler, _ := newMockDiscoveryScheduler(
		1,
		[]string{"c1", "c2"},
		[]string{"r1", "r2"},
	)

	expected := [][]string{
		{"c1"},
		{"r1"},
		{"c2"},
		{"r2"},
	}

	for i, ids := range expected {
		candidates, err := scheduler.Next(context.Background())

		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}

		assertDiscoveryIDs(t, candidates, ids...)
	}
}

func TestDiscoverySchedulerIndependentCursorWraparound(
	t *testing.T,
) {
	scheduler, _ := newMockDiscoveryScheduler(
		2,
		[]string{"c1", "c2"},
		[]string{"r1", "r2"},
	)

	expected := [][]string{
		{"c1", "r1"},
		{"c2", "r2"},
		{"c1", "r1"},
	}

	for i, ids := range expected {
		candidates, err := scheduler.Next(context.Background())

		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}

		assertDiscoveryIDs(t, candidates, ids...)
	}
}

func TestDiscoverySchedulerRedistributesWithoutDuplicates(
	t *testing.T,
) {
	scheduler, calls := newMockDiscoveryScheduler(
		4,
		[]string{"c1", "c2", "c3"},
		nil,
	)

	candidates, err := scheduler.Next(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	assertDiscoveryIDs(t, candidates, "c1", "c2", "c3")

	if scheduler.captureCursor != "c3" {
		t.Fatalf(
			"capture cursor = %q, want c3",
			scheduler.captureCursor,
		)
	}

	// The redistribution query must continue from c2.
	found := false

	for _, call := range *calls {
		if call.kind == postgresrepo.FlexSettlementCandidateCapture &&
			call.afterID == "c2" &&
			call.limit == 2 {
			found = true
		}
	}

	if !found {
		t.Fatal("expected forward-only capture redistribution")
	}

	// On the next call, the cursor may wrap at the category
	// boundary, but each individual batch must remain unique.
	candidates, err = scheduler.Next(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	assertDiscoveryIDs(t, candidates, "c1", "c2", "c3")
}

func TestDiscoverySchedulerPreservesCursorsOnQueryFailure(
	t *testing.T,
) {
	scheduler, _ := newMockDiscoveryScheduler(
		2,
		[]string{"c1", "c2"},
		[]string{"r1", "r2"},
	)

	scheduler.captureCursor = "c1"
	scheduler.recoveryCursor = "r1"

	expectedError := errors.New("simulated repository failure")

	originalQuery := scheduler.query

	scheduler.query = func(
		ctx context.Context,
		db *pgxpool.Pool,
		kind postgresrepo.FlexSettlementCandidateKind,
		afterID string,
		limit int,
	) ([]postgresrepo.FlexSettlementCandidate, error) {
		if kind == postgresrepo.FlexSettlementCandidateRecovery {
			return nil, expectedError
		}

		return originalQuery(ctx, db, kind, afterID, limit)
	}

	_, err := scheduler.Next(context.Background())

	if !errors.Is(err, expectedError) {
		t.Fatalf(
			"error = %v, want repository failure",
			err,
		)
	}

	if scheduler.captureCursor != "c1" ||
		scheduler.recoveryCursor != "r1" {
		t.Fatalf(
			"cursors changed after failure: capture=%q recovery=%q",
			scheduler.captureCursor,
			scheduler.recoveryCursor,
		)
	}

	if scheduler.preferRecovery {
		t.Fatal("category priority changed after failure")
	}
}

func TestDiscoverySchedulerBatchLimit(
	t *testing.T,
) {
	scheduler, _ := newMockDiscoveryScheduler(
		3,
		[]string{"c1", "c2", "c3", "c4"},
		[]string{"r1", "r2", "r3", "r4"},
	)

	for i := 0; i < 5; i++ {
		candidates, err := scheduler.Next(context.Background())

		if err != nil {
			t.Fatal(err)
		}

		if len(candidates) > 3 {
			t.Fatalf(
				"iteration %d returned %d candidates",
				i,
				len(candidates),
			)
		}

		seen := make(map[string]bool)

		for _, candidate := range candidates {
			if seen[candidate.SettlementID] {
				t.Fatalf(
					"iteration %d duplicated %s",
					i,
					candidate.SettlementID,
				)
			}

			seen[candidate.SettlementID] = true
		}
	}
}

func TestDiscoverySchedulerRejectsNilDatabase(
	t *testing.T,
) {
	_, err := NewDiscoveryScheduler(nil, 10)

	if err == nil {
		t.Fatal("expected nil database error")
	}
}

func TestDiscoverySchedulerRejectsNilReceiver(
	t *testing.T,
) {
	var scheduler *DiscoveryScheduler

	_, err := scheduler.Next(context.Background())

	if err == nil {
		t.Fatal("expected nil scheduler error")
	}
}
