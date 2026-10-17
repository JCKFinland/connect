package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestFlexAuthorizationLifecycle(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	assertRejected := func(t *testing.T, query string, args ...any) {
		t.Helper()

		_, err := f.DB.Exec(ctx, query, args...)
		if err == nil {
			t.Fatal("database accepted an invalid authorization operation")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("expected PostgreSQL constraint error, got: %v", err)
		}
		if pgErr.Code != "23514" {
			t.Fatalf("expected SQLSTATE 23514, got %s: %v", pgErr.Code, err)
		}
	}

	t.Run("pending cannot authorize directly", func(t *testing.T) {
		assertRejected(t, `
			UPDATE flex_authorizations
			SET status = 'AUTHORIZED',
			    provider_authorization_id = 'pi_direct',
			    authorized_at = clock_timestamp()
			WHERE id = $1
		`, authorization.ID)
	})

	t.Run("authorized requires provider identity", func(t *testing.T) {
		_, err := f.DB.Exec(ctx, `
			UPDATE flex_authorizations
			SET status = 'PROCESSING'
			WHERE id = $1
		`, authorization.ID)
		if err != nil {
			t.Fatal(err)
		}

		assertRejected(t, `
			UPDATE flex_authorizations
			SET status = 'AUTHORIZED',
			    authorized_at = clock_timestamp()
			WHERE id = $1
		`, authorization.ID)
	})

	t.Run("provider identity cannot be replaced", func(t *testing.T) {
		_, err := f.DB.Exec(ctx, `
			UPDATE flex_authorizations
			SET provider_authorization_id = 'pi_original'
			WHERE id = $1
		`, authorization.ID)
		if err != nil {
			t.Fatal(err)
		}

		assertRejected(t, `
			UPDATE flex_authorizations
			SET provider_authorization_id = 'pi_replacement'
			WHERE id = $1
		`, authorization.ID)
	})

	t.Run("processing can authorize", func(t *testing.T) {
		_, err := f.DB.Exec(ctx, `
			UPDATE flex_authorizations
			SET status = 'AUTHORIZED',
			    authorized_at = clock_timestamp()
			WHERE id = $1
		`, authorization.ID)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("authorized is terminal", func(t *testing.T) {
		assertRejected(t, `
			UPDATE flex_authorizations
			SET status = 'FAILED'
			WHERE id = $1
		`, authorization.ID)

		assertRejected(t, `
			UPDATE flex_authorizations
			SET status = 'CANCELLED'
			WHERE id = $1
		`, authorization.ID)
	})

	t.Run("authorized timestamp cannot change", func(t *testing.T) {
		assertRejected(t, `
			UPDATE flex_authorizations
			SET authorized_at = authorized_at + INTERVAL '1 second'
			WHERE id = $1
		`, authorization.ID)
	})
}

func TestFlexAuthorizationExpiredInitiationRejected(t *testing.T) {
	f := setupFlexAuthorizationTest(t)
	ctx := context.Background()

	authorization, err := createTestFlexAuthorization(ctx, f)
	if err != nil {
		t.Fatal(err)
	}

	// The immutable deadline cannot be rewritten. Simulate expiry
	// by moving the database session's clock forward within a transaction
	// rather than changing authorization financial identity.
	//
	// PostgreSQL does not support changing clock_timestamp() through
	// SET LOCAL, so this test uses a dedicated short-lived authorization
	// inserted with an already-expired initiation deadline.
	_, err = f.DB.Exec(ctx, `
		DELETE FROM flex_authorizations WHERE id = $1
	`, authorization.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.DB.Exec(ctx, `
		INSERT INTO flex_authorizations (
			auction_id,
			offer_id,
			passenger_id,
			amount,
			currency,
			idempotency_key,
			created_at,
			expires_at
		)
		VALUES (
			$1, $2, $3, 23.28, 'EUR', $4,
			clock_timestamp() - INTERVAL '10 minutes',
			clock_timestamp() - INTERVAL '5 minutes'
		)
	`,
		f.AuctionID,
		f.OfferID,
		f.PassengerID,
		"expired-initiation:"+f.AuctionID,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_authorizations
		SET status = 'PROCESSING'
		WHERE auction_id = $1
	`, f.AuctionID)

	if err == nil {
		t.Fatal("expired authorization entered PROCESSING")
	}

	_, err = f.DB.Exec(ctx, `
		UPDATE flex_authorizations
		SET status = 'EXPIRED'
		WHERE auction_id = $1
	`, f.AuctionID)
	if err != nil {
		t.Fatalf("expired PENDING authorization could not be closed: %v", err)
	}
}
