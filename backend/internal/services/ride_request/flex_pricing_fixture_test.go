package ride_request

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createFlexServicePricingFixture(
	t *testing.T,
	db *pgxpool.Pool,
) (string, string) {
	t.Helper()

	ctx := context.Background()
	categoryID := uuid.NewString()
	profileID := uuid.NewString()
	version := "flex-" + uuid.NewString()[:8]
	now := time.Now().UTC()

	var companyID string
	var branchID string

	err := db.QueryRow(ctx, `
		SELECT c.id, b.id
		FROM companies c
		JOIN branches b ON b.company_id = c.id
		WHERE c.deleted_at IS NULL
		  AND b.deleted_at IS NULL
		ORDER BY c.id, b.id
		LIMIT 1
	`).Scan(&companyID, &branchID)
	if err != nil {
		t.Fatalf("resolve FLEX fixture company and branch: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO service_categories (
			id, code, name, description,
			is_active, created_at, updated_at
		)
		VALUES (
			$1, $2, 'FLEX Service Pricing Test',
			'Disposable FLEX service integration fixture',
			TRUE, $3, $3
		)
	`, categoryID, "FLEX_"+uuid.NewString()[:8], now)
	if err != nil {
		t.Fatalf("create FLEX service category: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM service_categories WHERE id = $1`,
			categoryID,
		); err != nil {
			t.Errorf("cleanup FLEX service category: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
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
		t.Fatalf("create FLEX service pricing profile: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM fare_pricing_profiles WHERE id = $1`,
			profileID,
		); err != nil {
			t.Errorf("cleanup FLEX service pricing profile: %v", err)
		}
	})

	return profileID, version
}
