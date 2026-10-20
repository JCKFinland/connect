package testutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateAuthorizedCustomerFixture creates a verified integration-test
// customer with the CUSTOMER role and returns dependency-safe cleanup.
func CreateAuthorizedCustomerFixture(
	ctx context.Context,
	db *pgxpool.Pool,
) (string, func(context.Context) error, error) {
	if db == nil {
		return "", nil, fmt.Errorf("database pool is required")
	}

	customerID, cleanupCustomer, err := CreateCustomerFixture(ctx, db)
	if err != nil {
		return "", nil, err
	}

	var customerRoleID string

	err = db.QueryRow(
		ctx,
		`SELECT id FROM roles WHERE name = 'CUSTOMER'`,
	).Scan(&customerRoleID)

	if err != nil {
		cleanupErr := cleanupCustomer(context.Background())
		return "", nil, errors.Join(
			fmt.Errorf("resolve CUSTOMER role: %w", err),
			cleanupErr,
		)
	}

	_, err = db.Exec(
		ctx,
		`INSERT INTO user_roles (user_id, role_id)
         VALUES ($1, $2)`,
		customerID,
		customerRoleID,
	)

	if err != nil {
		cleanupErr := cleanupCustomer(context.Background())
		return "", nil, errors.Join(
			fmt.Errorf("assign CUSTOMER role: %w", err),
			cleanupErr,
		)
	}

	cleanup := func(cleanupCtx context.Context) error {
		_, roleErr := db.Exec(
			cleanupCtx,
			`DELETE FROM user_roles
             WHERE user_id = $1 AND role_id = $2`,
			customerID,
			customerRoleID,
		)

		if roleErr != nil {
			return fmt.Errorf(
				"cleanup CUSTOMER role: %w",
				roleErr,
			)
		}

		return cleanupCustomer(cleanupCtx)
	}

	return customerID, cleanup, nil
}
