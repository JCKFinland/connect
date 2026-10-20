package testutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AttachDispatchPricingFixture creates pricing dependencies for an
// existing integration-test ride request.
func AttachDispatchPricingFixture(
	ctx context.Context,
	db *pgxpool.Pool,
	rideRequestID string,
	companyID string,
) (func(context.Context) error, error) {
	if rideRequestID == "" || companyID == "" {
		return nil, errors.New("ride and company IDs are required")
	}

	categoryID := uuid.NewString()
	profileID := uuid.NewString()
	version := "dispatch-test-" + uuid.NewString()
	code := "DISPATCH_" + uuid.NewString()[:8]
	effectiveFrom := time.Now().UTC().Add(-time.Hour)

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin fixture transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(ctx, `
        INSERT INTO service_categories (
            id, code, name, description,
            is_active, created_at, updated_at
        )
        VALUES (
            $1, $2, 'Dispatch Test Category',
            'Isolated dispatch pricing fixture',
            TRUE, NOW(), NOW()
        )
    `, categoryID, code)
	if err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO fare_pricing_profiles (
            id, company_id, branch_id, service_category_id,
            version, currency, base_fare,
            distance_rate_per_km, time_rate_per_minute,
            waiting_rate_per_minute, booking_fee,
            surge_multiplier, effective_from,
            effective_to, is_active, created_at
        )
        VALUES (
            $1, $2, NULL, $3,
            $4, 'EUR', 5.00,
            1.50, 0.50,
            0.50, 0.00,
            1.00, $5,
            NULL, TRUE, $5
        )
    `, profileID, companyID, categoryID, version, effectiveFrom)
	if err != nil {
		return nil, fmt.Errorf("create pricing profile: %w", err)
	}

	tag, err := tx.Exec(ctx, `
        UPDATE ride_requests
        SET service_category_id = $2,
            updated_at = NOW()
        WHERE id = $1
          AND service_category_id IS NULL
    `, rideRequestID, categoryID)
	if err != nil {
		return nil, fmt.Errorf("attach category: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return nil, errors.New("ride missing or already categorized")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit fixture: %w", err)
	}

	cleanup := func(cleanupCtx context.Context) error {
		tx, err := db.Begin(cleanupCtx)
		if err != nil {
			return err
		}
		defer tx.Rollback(context.Background())

		if _, err := tx.Exec(cleanupCtx, `
            UPDATE ride_requests
            SET service_category_id = NULL
            WHERE id = $1 AND service_category_id = $2
        `, rideRequestID, categoryID); err != nil {
			return err
		}

		if _, err := tx.Exec(cleanupCtx, `
            DELETE FROM fare_pricing_profiles WHERE id = $1
        `, profileID); err != nil {
			return err
		}

		if _, err := tx.Exec(cleanupCtx, `
            DELETE FROM service_categories WHERE id = $1
        `, categoryID); err != nil {
			return err
		}

		return tx.Commit(cleanupCtx)
	}

	return cleanup, nil
}
