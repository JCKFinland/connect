package trip

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestTripTransactionalTenantAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("refusing tenant authorization test outside integration database")
	}

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
		t.Fatalf("load configuration: %v", err)
	}
	if cfg.Database.Name != "connect_integration_test" {
		t.Fatalf("unsafe database: %s", cfg.Database.Name)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	defer db.Close()

	userID := uuid.NewString()
	companyOneID := uuid.NewString()
	companyTwoID := uuid.NewString()

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		statements := []struct {
			query string
			args  []any
		}{
			{
				`DELETE FROM company_memberships WHERE user_id = $1`,
				[]any{userID},
			},
			{
				`DELETE FROM companies WHERE id = ANY($1::uuid[])`,
				[]any{[]string{companyOneID, companyTwoID}},
			},
			{
				`DELETE FROM users WHERE id = $1`,
				[]any{userID},
			},
		}

		for _, statement := range statements {
			if _, cleanupErr := db.Exec(
				cleanupCtx,
				statement.query,
				statement.args...,
			); cleanupErr != nil {
				t.Errorf("cleanup authorization fixture: %v", cleanupErr)
			}
		}
	}()

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash, first_name, last_name
        )
        VALUES ($1, $2, 'test-hash', 'Tenant', 'Authority')
    `, userID, "tenant-authority-"+userID+"@example.test")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	for index, companyID := range []string{companyOneID, companyTwoID} {
		_, err = db.Exec(ctx, `
            INSERT INTO companies (
                id, name, legal_name, business_id,
                email, country_code, timezone
            )
            VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
        `,
			companyID,
			"Tenant Authority Company "+companyID[:8],
			"Tenant Authority Test Oy",
			"AUTH-"+companyID[:8],
			companyID+"@example.test",
		)
		if err != nil {
			t.Fatalf("create company %d: %v", index, err)
		}
	}

	_, err = db.Exec(ctx, `
        INSERT INTO company_memberships (user_id, company_id)
        VALUES ($1, $2)
    `, userID, companyOneID)
	if err != nil {
		t.Fatalf("create authorized membership: %v", err)
	}

	service := &tripService{}

	checkOperational := func(
		name string,
		companyID string,
		roles []string,
		expected bool,
	) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				t.Fatalf("begin authorization transaction: %v", err)
			}
			defer func() {
				_ = tx.Rollback(context.Background())
			}()

			allowed, err := service.hasOperationalCompanyAuthorityForUpdate(
				ctx, tx, userID, companyID, roles,
			)
			if err != nil {
				t.Fatalf("check operational authority: %v", err)
			}
			if allowed != expected {
				t.Fatalf("expected authority %t, got %t", expected, allowed)
			}
		})
	}

	checkOperational(
		"company administrator authorized",
		companyOneID,
		[]string{"COMPANY_ADMIN"},
		true,
	)

	checkOperational(
		"dispatcher authorized",
		companyOneID,
		[]string{"DISPATCHER"},
		true,
	)

	checkOperational(
		"cross company administrator denied",
		companyTwoID,
		[]string{"COMPANY_ADMIN"},
		false,
	)

	checkOperational(
		"unprivileged role denied",
		companyOneID,
		[]string{"CUSTOMER"},
		false,
	)

	checkOperational(
		"system administrator global authority",
		companyTwoID,
		[]string{"SYSTEM_ADMIN"},
		true,
	)

	t.Run("assigned driver retains lifecycle authority", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatalf("begin driver authorization transaction: %v", err)
		}
		defer func() {
			_ = tx.Rollback(context.Background())
		}()

		assignedTrip := &models.Trip{
			CompanyID: companyTwoID,
			DriverID:  userID,
		}

		allowed, err := service.canUpdateTripStatusAuthorizedForUpdate(
			ctx, tx, []string{"DRIVER"}, userID, assignedTrip,
		)
		if err != nil {
			t.Fatalf("check assigned driver authority: %v", err)
		}
		if !allowed {
			t.Fatal("assigned driver must retain lifecycle authority")
		}
	})

	t.Run("unassigned driver denied", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatalf("begin driver authorization transaction: %v", err)
		}
		defer func() {
			_ = tx.Rollback(context.Background())
		}()

		assignedTrip := &models.Trip{
			CompanyID: companyTwoID,
			DriverID:  uuid.NewString(),
		}

		allowed, err := service.canUpdateTripStatusAuthorizedForUpdate(
			ctx, tx, []string{"DRIVER"}, userID, assignedTrip,
		)
		if err != nil {
			t.Fatalf("check unassigned driver authority: %v", err)
		}
		if allowed {
			t.Fatal("unassigned driver must not receive lifecycle authority")
		}
	})

	_, err = db.Exec(ctx, `
        DELETE FROM company_memberships
        WHERE user_id = $1 AND company_id = $2
    `, userID, companyOneID)
	if err != nil {
		t.Fatalf("revoke membership: %v", err)
	}

	checkOperational(
		"revoked company administrator denied",
		companyOneID,
		[]string{"COMPANY_ADMIN"},
		false,
	)

	checkOperational(
		"revoked dispatcher denied",
		companyOneID,
		[]string{"DISPATCHER"},
		false,
	)
}
