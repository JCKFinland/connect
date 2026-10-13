package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// createFlexPricingFixture creates a disposable pricing profile inside
// the caller's transaction. The caller owns transaction cleanup.
func createFlexPricingFixture(
	ctx context.Context,
	tx pgx.Tx,
	companyID string,
	branchID string,
) (string, string, error) {
	categoryID := uuid.NewString()
	profileID := uuid.NewString()
	version := "flex-" + uuid.NewString()[:8]
	now := time.Now().UTC()

	_, err := tx.Exec(ctx, `
		INSERT INTO service_categories (
			id, code, name, description,
			is_active, created_at, updated_at
		)
		VALUES (
			$1, $2, 'FLEX Pricing Test',
			'Disposable FLEX pricing integration fixture',
			TRUE, $3, $3
		)
	`, categoryID, "FLEX_"+uuid.NewString()[:8], now)
	if err != nil {
		return "", "", fmt.Errorf(
			"create FLEX fixture service category: %w", err,
		)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO fare_pricing_profiles (
			id, company_id, branch_id,
			service_category_id, version, currency,
			base_fare, distance_rate_per_km,
			time_rate_per_minute, waiting_rate_per_minute,
			booking_fee, surge_multiplier,
			effective_from, effective_to,
			is_active, created_at
		)
		VALUES (
			$1, $2, $3, $4, $5, 'EUR',
			4.90, 1.50, 0.25, 0.50,
			2.00, 1.00,
			$6, NULL, TRUE, $6
		)
	`, profileID, companyID, branchID, categoryID, version, now)
	if err != nil {
		return "", "", fmt.Errorf(
			"create FLEX fixture pricing profile: %w", err,
		)
	}

	return profileID, version, nil
}
