package testutil

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateCustomerFixture creates an isolated customer for integration tests.
// The caller must invoke cleanup after removing dependent records.
func CreateCustomerFixture(
	ctx context.Context,
	db *pgxpool.Pool,
) (string, func(context.Context) error, error) {
	if db == nil {
		return "", nil, fmt.Errorf("database pool is required")
	}

	customerID := uuid.NewString()
	customerEmail := "customer-" + customerID + "@example.invalid"

	_, err := db.Exec(
		ctx,
		`INSERT INTO users (
            id, email, password_hash, first_name, last_name,
            is_active, is_verified
        ) VALUES (
            $1, $2, $3, 'CONNECT', 'Integration Customer', TRUE, TRUE
        )`,
		customerID,
		customerEmail,
		"test-account-no-login",
	)
	if err != nil {
		return "", nil, fmt.Errorf("create test customer: %w", err)
	}

	cleanup := func(cleanupCtx context.Context) error {
		_, err := db.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1`,
			customerID,
		)
		if err != nil {
			return fmt.Errorf("delete test customer: %w", err)
		}
		return nil
	}

	return customerID, cleanup, nil
}
