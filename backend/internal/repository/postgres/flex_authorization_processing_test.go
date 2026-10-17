package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestClaimFlexAuthorizationProcessingSuccess(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	updated, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx,
			f.DB,
			authorization.ID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("expected first processing claim to succeed")
	}
	if updated == nil || updated.Status != "PROCESSING" {
		t.Fatalf("unexpected authorization result: %+v", updated)
	}
	if updated.ID != authorization.ID ||
		updated.IdempotencyKey != authorization.IdempotencyKey {
		t.Fatal("processing claim changed authorization identity")
	}

	var persistedStatus string
	err = f.DB.QueryRow(ctx, `
		SELECT status
		FROM flex_authorizations
		WHERE id = $1
	`, authorization.ID).Scan(&persistedStatus)
	if err != nil {
		t.Fatal(err)
	}
	if persistedStatus != "PROCESSING" {
		t.Fatalf("unexpected persisted status: %s", persistedStatus)
	}
}

func TestClaimFlexAuthorizationProcessingIdempotent(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	first, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}

	second, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("idempotent retry claimed authorization twice")
	}
	if second.ID != first.ID ||
		second.IdempotencyKey != first.IdempotencyKey ||
		second.Status != "PROCESSING" {
		t.Fatal("idempotent retry returned inconsistent authorization")
	}
}

func TestClaimFlexAuthorizationProcessingConcurrent(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 8

	type result struct {
		authorizationID string
		key             string
		status          string
		claimed         bool
		err             error
	}

	results := make(chan result, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			updated, claimed, err :=
				postgresrepo.ClaimFlexAuthorizationProcessing(
					context.Background(),
					f.DB,
					authorization.ID,
				)

			r := result{
				claimed: claimed,
				err:     err,
			}
			if updated != nil {
				r.authorizationID = updated.ID
				r.key = updated.IdempotencyKey
				r.status = updated.Status
			}
			results <- r
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	claimCount := 0
	resultCount := 0

	for r := range results {
		resultCount++

		if r.err != nil {
			t.Fatalf("concurrent claim failed: %v", r.err)
		}
		if r.authorizationID != authorization.ID ||
			r.key != authorization.IdempotencyKey ||
			r.status != "PROCESSING" {
			t.Fatalf("inconsistent concurrent result: %+v", r)
		}
		if r.claimed {
			claimCount++
		}
	}

	if resultCount != workers {
		t.Fatalf("expected %d results, got %d", workers, resultCount)
	}
	if claimCount != 1 {
		t.Fatalf("expected exactly one claim, got %d", claimCount)
	}
}

func TestClaimFlexAuthorizationProcessingMissing(t *testing.T) {
	f := setupFlexAuthorizationTest(t)

	updated, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			context.Background(),
			f.DB,
			uuid.NewString(),
		)

	if !errors.Is(err, postgresrepo.ErrFlexAuthorizationNotFound) {
		t.Fatalf("expected not-found error, got: %v", err)
	}
	if claimed || updated != nil {
		t.Fatal("missing authorization was claimed")
	}
}

func TestClaimFlexAuthorizationProcessingTerminal(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_authorizations
		SET status = 'CANCELLED'
		WHERE id = $1
	`, authorization.ID)
	if err != nil {
		t.Fatal(err)
	}

	updated, claimed, err :=
		postgresrepo.ClaimFlexAuthorizationProcessing(
			ctx, f.DB, authorization.ID,
		)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("terminal authorization was claimed")
	}
	if updated == nil || updated.Status != "CANCELLED" {
		t.Fatalf("unexpected terminal result: %+v", updated)
	}
}
