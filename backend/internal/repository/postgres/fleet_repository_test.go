package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestFleetRepositoryEnforcesCompanyMembershipReadAuthority(
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

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})

	memberUserID := uuid.NewString()
	noMembershipUserID := uuid.NewString()
	companyOneID := uuid.NewString()
	companyTwoID := uuid.NewString()
	branchOneID := uuid.NewString()
	branchTwoID := uuid.NewString()
	fleetOneID := uuid.NewString()
	fleetTwoID := uuid.NewString()

	createUser := func(
		userID string,
		label string,
	) {
		t.Helper()

		_, err := tx.Exec(
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
			label+"-"+userID+"@example.test",
			"test-password-hash",
			"Fleet",
			"ReadAuthority",
		)
		if err != nil {
			t.Fatalf("create user %s: %v", label, err)
		}
	}

	createCompanyHierarchy := func(
		companyID string,
		branchID string,
		fleetID string,
		label string,
	) {
		t.Helper()

		_, err := tx.Exec(
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
				VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
			`,
			companyID,
			"Fleet Read "+label,
			"Fleet Read "+label+" Oy",
			"FR-"+companyID[:8],
			companyID+"@example.test",
		)
		if err != nil {
			t.Fatalf("create company %s: %v", label, err)
		}

		_, err = tx.Exec(
			ctx,
			`
				INSERT INTO branches (
					id,
					company_id,
					code,
					name
				)
				VALUES ($1, $2, $3, $4)
			`,
			branchID,
			companyID,
			"BR-"+branchID[:8],
			"Fleet Read Branch "+label,
		)
		if err != nil {
			t.Fatalf("create branch %s: %v", label, err)
		}

		_, err = tx.Exec(
			ctx,
			`
				INSERT INTO fleets (
					id,
					company_id,
					branch_id,
					code,
					name,
					description
				)
				VALUES ($1, $2, $3, $4, $5, $6)
			`,
			fleetID,
			companyID,
			branchID,
			"FL-"+fleetID[:8],
			"Fleet Read Fleet "+label,
			"Fleet read authority test "+label,
		)
		if err != nil {
			t.Fatalf("create fleet %s: %v", label, err)
		}
	}

	createUser(memberUserID, "member")
	createUser(noMembershipUserID, "no-membership")

	createCompanyHierarchy(
		companyOneID,
		branchOneID,
		fleetOneID,
		"One",
	)
	createCompanyHierarchy(
		companyTwoID,
		branchTwoID,
		fleetTwoID,
		"Two",
	)

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO company_memberships (
				user_id,
				company_id
			)
			VALUES ($1, $2)
		`,
		memberUserID,
		companyOneID,
	)
	if err != nil {
		t.Fatalf("create company membership: %v", err)
	}

	repo := &FleetRepository{
		db: tx,
	}

	ownFleet, err := repo.GetByIDForCompanyMember(
		ctx,
		memberUserID,
		fleetOneID,
	)
	if err != nil {
		t.Fatalf("get own-company fleet: %v", err)
	}
	if ownFleet.ID != fleetOneID {
		t.Fatalf(
			"expected own-company fleet %s, got %s",
			fleetOneID,
			ownFleet.ID,
		)
	}
	if ownFleet.CompanyID != companyOneID {
		t.Fatalf(
			"expected company %s, got %s",
			companyOneID,
			ownFleet.CompanyID,
		)
	}

	_, err = repo.GetByIDForCompanyMember(
		ctx,
		memberUserID,
		fleetTwoID,
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound for cross-company fleet, got %v",
			err,
		)
	}

	memberFleets, err := repo.ListForCompanyMember(
		ctx,
		memberUserID,
	)
	if err != nil {
		t.Fatalf("list member fleets: %v", err)
	}
	if len(memberFleets) != 1 {
		t.Fatalf(
			"expected exactly 1 membership-scoped fleet, got %d",
			len(memberFleets),
		)
	}
	if memberFleets[0].ID != fleetOneID {
		t.Fatalf(
			"expected only fleet %s, got %s",
			fleetOneID,
			memberFleets[0].ID,
		)
	}
	if memberFleets[0].CompanyID != companyOneID {
		t.Fatalf(
			"expected only company %s, got %s",
			companyOneID,
			memberFleets[0].CompanyID,
		)
	}

	noMembershipFleets, err := repo.ListForCompanyMember(
		ctx,
		noMembershipUserID,
	)
	if err != nil {
		t.Fatalf("list fleets without membership: %v", err)
	}
	if len(noMembershipFleets) != 0 {
		t.Fatalf(
			"user without company membership must see 0 fleets, got %d",
			len(noMembershipFleets),
		)
	}

	globalFleets, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list global fleets: %v", err)
	}

	foundOne := false
	foundTwo := false

	for _, fleet := range globalFleets {
		switch fleet.ID {
		case fleetOneID:
			foundOne = true
		case fleetTwoID:
			foundTwo = true
		}
	}

	if !foundOne || !foundTwo {
		t.Fatalf(
			"global repository read must retain both test fleets: fleetOne=%t fleetTwo=%t",
			foundOne,
			foundTwo,
		)
	}
}
