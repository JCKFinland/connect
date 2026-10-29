package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
)

func TestCompanyMembershipExistsForShareBlocksRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

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

	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("refusing membership lock test outside connect_integration_test")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if cfg.Database.Name != "connect_integration_test" {
		t.Fatalf("unsafe configured database: %s", cfg.Database.Name)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	userID := uuid.NewString()
	companyID := uuid.NewString()

	cleanup := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, statement := range []string{
			`DELETE FROM company_memberships WHERE user_id = $1`,
			`DELETE FROM companies WHERE id = $1`,
			`DELETE FROM users WHERE id = $1`,
		} {
			arg := userID
			if statement == `DELETE FROM companies WHERE id = $1` {
				arg = companyID
			}
			if _, cleanupErr := db.Exec(cleanupCtx, statement, arg); cleanupErr != nil {
				t.Errorf("cleanup fixture: %v", cleanupErr)
			}
		}
	}
	defer cleanup()

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash, first_name, last_name
        )
        VALUES ($1, $2, $3, 'Membership', 'Lock Test')
    `, userID, "membership-lock-"+userID+"@example.test", "test-hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, business_id,
            email, country_code, timezone
        )
        VALUES (
            $1, 'Membership Lock Test',
            'Membership Lock Test Oy', $2, $3,
            'FI', 'Europe/Helsinki'
        )
    `, companyID, "TEST-"+companyID[:8], companyID+"@example.test")
	if err != nil {
		t.Fatalf("create company: %v", err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO company_memberships (user_id, company_id)
        VALUES ($1, $2)
    `, userID, companyID)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin authorization transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	repo := NewCompanyMembershipRepository(tx)

	authorized, err := repo.ExistsForShare(ctx, userID, companyID)
	if err != nil {
		t.Fatalf("acquire membership share lock: %v", err)
	}
	if !authorized {
		t.Fatal("expected existing membership to authorize")
	}

	// A separate connection must not be able to revoke membership
	// while the authorization transaction holds FOR SHARE.
	// Use a separate PostgreSQL transaction with a server-side
	// lock timeout. A failed DELETE must be rolled back explicitly.
	revokeTx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin revocation transaction: %v", err)
	}

	_, err = revokeTx.Exec(ctx, `SET LOCAL lock_timeout = '300ms'`)
	if err != nil {
		_ = revokeTx.Rollback(context.Background())
		t.Fatalf("configure revocation lock timeout: %v", err)
	}

	_, err = revokeTx.Exec(ctx, `
        DELETE FROM company_memberships
        WHERE user_id = $1 AND company_id = $2
    `, userID, companyID)

	if err == nil {
		_ = revokeTx.Rollback(context.Background())
		t.Fatal("membership deletion unexpectedly bypassed FOR SHARE")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		_ = revokeTx.Rollback(context.Background())
		t.Fatalf(
			"expected PostgreSQL lock_not_available (55P03), got %v",
			err,
		)
	}

	if err := revokeTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback blocked revocation transaction: %v", err)
	}

	// The blocked deletion must not have removed the membership.
	stillExists, err := NewCompanyMembershipRepository(db).
		Exists(ctx, userID, companyID)
	if err != nil {
		t.Fatalf("verify membership before authorization commit: %v", err)
	}
	if !stillExists {
		t.Fatal("membership disappeared while authorization lock was held")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit authorization transaction: %v", err)
	}

	tag, err := db.Exec(ctx, `
        DELETE FROM company_memberships
        WHERE user_id = $1 AND company_id = $2
    `, userID, companyID)
	if err != nil {
		t.Fatalf("revoke membership after commit: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected one revoked membership, got %d", tag.RowsAffected())
	}

	verifyTx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin post-revocation transaction: %v", err)
	}
	defer func() {
		_ = verifyTx.Rollback(context.Background())
	}()

	authorized, err = NewCompanyMembershipRepository(verifyTx).
		ExistsForShare(ctx, userID, companyID)
	if err != nil {
		t.Fatalf("verify revoked membership: %v", err)
	}
	if authorized {
		t.Fatal("revoked membership must not authorize")
	}

	if err := verifyTx.Commit(ctx); err != nil {
		t.Fatalf("commit verification transaction: %v", err)
	}
}
