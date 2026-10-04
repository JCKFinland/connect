package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestCompanyMembershipRepositoryEnforcesTenantAuthority(
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

	userID := uuid.NewString()
	companyOneID := uuid.NewString()
	companyTwoID := uuid.NewString()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO users (
				id,
				email,
				password_hash,
				first_name,
				last_name
			)
			VALUES ($1, $2, $3, $4, $5)
		`,
		userID,
		"membership-"+userID+"@example.test",
		"test-password-hash",
		"Membership",
		"Authority",
	)
	if err != nil {
		t.Fatalf("create membership test user: %v", err)
	}

	createCompany := func(
		companyID string,
		name string,
	) {
		t.Helper()

		_, err := db.Exec(
			ctx,
			`
				INSERT INTO companies (
					id,
					name,
					legal_name,
					business_id,
					email,
					country_code,
					timezone
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`,
			companyID,
			name,
			name+" Oy",
			"TEST-"+companyID[:8],
			companyID+"@example.test",
			"FI",
			"Europe/Helsinki",
		)
		if err != nil {
			t.Fatalf("create company %s: %v", companyID, err)
		}
	}

	createCompany(companyOneID, "Membership Company One")
	createCompany(companyTwoID, "Membership Company Two")

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM company_memberships WHERE user_id = $1`,
			userID,
		); cleanupErr != nil {
			t.Logf("cleanup company memberships: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM companies WHERE id = ANY($1::uuid[])`,
			[]string{companyOneID, companyTwoID},
		); cleanupErr != nil {
			t.Logf("cleanup companies: %v", cleanupErr)
		}

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1`,
			userID,
		); cleanupErr != nil {
			t.Logf("cleanup user: %v", cleanupErr)
		}
	})

	repo := NewCompanyMembershipRepository(db)

	first := &models.CompanyMembership{
		UserID:    userID,
		CompanyID: companyOneID,
	}

	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first company membership: %v", err)
	}

	if first.CreatedAt.IsZero() {
		t.Fatal("expected repository to return membership created_at")
	}

	exists, err := repo.Exists(
		ctx,
		userID,
		companyOneID,
	)
	if err != nil {
		t.Fatalf("check exact membership: %v", err)
	}
	if !exists {
		t.Fatal("expected exact user/company membership to exist")
	}

	exists, err = repo.Exists(
		ctx,
		uuid.NewString(),
		companyOneID,
	)
	if err != nil {
		t.Fatalf("check unrelated user membership: %v", err)
	}
	if exists {
		t.Fatal("unrelated user must not inherit company membership")
	}

	exists, err = repo.Exists(
		ctx,
		userID,
		companyTwoID,
	)
	if err != nil {
		t.Fatalf("check unrelated company membership: %v", err)
	}
	if exists {
		t.Fatal("membership in one company must not authorize another company")
	}

	second := &models.CompanyMembership{
		UserID:    userID,
		CompanyID: companyTwoID,
	}

	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second company membership: %v", err)
	}

	memberships, err := repo.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("list user memberships: %v", err)
	}
	if len(memberships) != 2 {
		t.Fatalf(
			"expected user to have 2 company memberships, got %d",
			len(memberships),
		)
	}

	duplicate := &models.CompanyMembership{
		UserID:    userID,
		CompanyID: companyOneID,
	}

	err = repo.Create(ctx, duplicate)
	if !errors.Is(
		err,
		repository.ErrCompanyMembershipAlreadyExists,
	) {
		t.Fatalf(
			"expected duplicate membership error, got %v",
			err,
		)
	}

	assertRestricted := func(
		name string,
		query string,
		arg string,
	) {
		t.Helper()

		_, err := db.Exec(ctx, query, arg)
		if err == nil {
			t.Fatalf(
				"expected %s deletion to be restricted by membership authority",
				name,
			)
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf(
				"expected PostgreSQL error for restricted %s deletion, got %v",
				name,
				err,
			)
		}

		if pgErr.Code != "23001" {
			t.Fatalf(
				"expected restrict_violation for restricted %s deletion, got PostgreSQL code %s",
				name,
				pgErr.Code,
			)
		}
	}

	assertRestricted(
		"user",
		`DELETE FROM users WHERE id = $1`,
		userID,
	)

	assertRestricted(
		"company",
		`DELETE FROM companies WHERE id = $1`,
		companyOneID,
	)
}
