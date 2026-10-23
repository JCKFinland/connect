package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestFlexAuthorizationRecoveryDiscovery(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement test database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must remain disabled")
	}

	ctx := context.Background()

	type fixtureCase struct {
		name       string
		status     string
		providerID string
		age        time.Duration
	}

	cases := []fixtureCase{
		{"oldest", "PROCESSING", "pi_recovery_oldest", 40 * time.Minute},
		{"second", "PROCESSING", "pi_recovery_second", 30 * time.Minute},
		{"third", "PROCESSING", "pi_recovery_third", 20 * time.Minute},
		{"fresh", "PROCESSING", "pi_recovery_fresh", time.Minute},
		{"unknown", "PROCESSING", "", 50 * time.Minute},
		{"pending", "PENDING", "", 50 * time.Minute},
	}

	type createdCase struct {
		name string
		id   string
	}

	var created []createdCase

	for _, tc := range cases {
		f := setupFlexAuthorizationTest(t)

		var databaseName string
		if err := f.DB.QueryRow(
			ctx, "SELECT current_database()",
		).Scan(&databaseName); err != nil {
			t.Fatal(err)
		}
		if databaseName != "connect_settlement_test" {
			t.Fatalf("unsafe database: %s", databaseName)
		}

		auth, err := createTestFlexAuthorization(ctx, f)
		if err != nil {
			t.Fatal(err)
		}

		if tc.status == "PROCESSING" {
			_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
				ctx, f.DB, auth.ID,
			)
			if err != nil || !claimed {
				t.Fatalf("claim %s: claimed=%v err=%v",
					tc.name, claimed, err)
			}
		}

		var providerID any
		if tc.providerID != "" {
			providerID = tc.providerID + "_" + auth.ID
		}

		tag, err := f.DB.Exec(ctx, `
			UPDATE flex_authorizations
			SET provider_authorization_id = $2,
			    updated_at = clock_timestamp() -
			        ($3::double precision * INTERVAL '1 second')
			WHERE id = $1
			  AND status = $4
		`,
			auth.ID,
			providerID,
			tc.age.Seconds(),
			tc.status,
		)
		if err != nil {
			t.Fatalf("prepare %s: %v", tc.name, err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("fixture update missing: %s", tc.name)
		}

		created = append(created, createdCase{
			name: tc.name,
			id:   auth.ID,
		})
	}

	f := setupFlexAuthorizationTest(t)

	var databaseName string
	if err := f.DB.QueryRow(
		ctx, "SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	lookup := make(map[string]string)
	for _, item := range created {
		lookup[item.name] = item.id
	}

	discover := func(age time.Duration, limit int) []string {
		t.Helper()

		result, err := postgresrepo.DiscoverFlexAuthorizationRecoveryCandidates(
			ctx, f.DB, age, limit,
		)
		if err != nil {
			t.Fatal(err)
		}

		owned := make(map[string]bool)
		for _, item := range created {
			owned[item.id] = true
		}

		var ids []string
		for _, candidate := range result {
			if owned[candidate.AuthorizationID] {
				ids = append(ids, candidate.AuthorizationID)
			}
		}

		return ids
	}

	ids := discover(15*time.Minute, 100)

	if len(ids) != 3 {
		t.Fatalf("eligible fixture count = %d, want 3", len(ids))
	}

	for i, name := range []string{"oldest", "second", "third"} {
		if ids[i] != lookup[name] {
			t.Fatalf(
				"candidate %d = %s, want %s",
				i, ids[i], lookup[name],
			)
		}
	}

	ids = discover(35*time.Minute, 100)
	if len(ids) != 1 || ids[0] != lookup["oldest"] {
		t.Fatalf("35-minute threshold selected unexpected fixtures: %v", ids)
	}

	ids = discover(60*time.Minute, 100)
	if len(ids) != 0 {
		t.Fatalf("60-minute threshold selected fixtures: %v", ids)
	}

	for _, item := range created {
		var status string
		var providerID *string

		err := f.DB.QueryRow(ctx, `
			SELECT status, provider_authorization_id
			FROM flex_authorizations
			WHERE id = $1
		`, item.id).Scan(&status, &providerID)
		if err != nil {
			t.Fatal(err)
		}

		if status != "PROCESSING" && status != "PENDING" {
			t.Fatalf("discovery changed status for %s: %s",
				item.name, status)
		}

		if item.name == "unknown" && providerID != nil {
			t.Fatal("discovery changed unknown provider identity")
		}
	}

	if _, err := postgresrepo.DiscoverFlexAuthorizationRecoveryCandidates(
		ctx, f.DB, 0, 10,
	); err == nil {
		t.Fatal("zero stale threshold accepted")
	}

	if _, err := postgresrepo.DiscoverFlexAuthorizationRecoveryCandidates(
		ctx, nil, time.Minute, 10,
	); err == nil {
		t.Fatal("nil database accepted")
	}
}

// TestFlexAuthorizationRecoveryDiscoveryBatchLimits verifies that
// the repository applies its batch limits to the complete result,
// not merely to records belonging to a particular test fixture.
func TestFlexAuthorizationRecoveryDiscoveryBatchLimits(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement test database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must remain disabled")
	}

	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	var databaseName string
	if err := f.DB.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
		ctx, f.DB, authorization.ID,
	)
	if err != nil || !claimed {
		t.Fatalf(
			"claim authorization: claimed=%v err=%v",
			claimed, err,
		)
	}

	tag, err := f.DB.Exec(ctx, `
		UPDATE flex_authorizations
		SET provider_authorization_id = $2,
		    updated_at = clock_timestamp() - INTERVAL '2 hours'
		WHERE id = $1
		  AND status = 'PROCESSING'
	`,
		authorization.ID,
		"pi_batch_limit_"+authorization.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("failed to prepare batch-limit fixture")
	}

	discover := func(batchSize int) []postgresrepo.FlexAuthorizationRecoveryCandidate {
		t.Helper()

		candidates, err :=
			postgresrepo.DiscoverFlexAuthorizationRecoveryCandidates(
				ctx,
				f.DB,
				15*time.Minute,
				batchSize,
			)
		if err != nil {
			t.Fatal(err)
		}

		return candidates
	}

	one := discover(1)
	if len(one) > 1 {
		t.Fatalf("batch size 1 returned %d candidates", len(one))
	}

	defaultBatch := discover(0)
	if len(defaultBatch) >
		postgresrepo.DefaultFlexAuthorizationRecoveryBatchSize {
		t.Fatalf(
			"default batch returned %d candidates",
			len(defaultBatch),
		)
	}

	maximum := discover(1000000)
	if len(maximum) >
		postgresrepo.MaxFlexAuthorizationRecoveryBatchSize {
		t.Fatalf(
			"maximum batch returned %d candidates",
			len(maximum),
		)
	}

	first := discover(100)
	second := discover(100)

	if len(first) != len(second) {
		t.Fatal("repeated discovery returned different result lengths")
	}

	for i := range first {
		if first[i].AuthorizationID != second[i].AuthorizationID {
			t.Fatalf(
				"discovery ordering changed at index %d",
				i,
			)
		}
	}

	found := false
	for _, candidate := range maximum {
		if candidate.AuthorizationID == authorization.ID {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("known-provider stale fixture missing from discovery")
	}

	var status string
	var providerID string
	var updatedAt time.Time

	if err := f.DB.QueryRow(ctx, `
		SELECT status, provider_authorization_id, updated_at
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(
		&status,
		&providerID,
		&updatedAt,
	); err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		providerID != "pi_batch_limit_"+authorization.ID ||
		updatedAt.After(time.Now().Add(-time.Hour)) {
		t.Fatal("discovery modified authorization financial state")
	}
}
