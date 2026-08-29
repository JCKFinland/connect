package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *FarePricingProfileRepository) ListByCompanyID(
	ctx context.Context,
	companyID string,
	activeOnly bool,
) ([]*models.FarePricingProfile, error) {
	query := `
		SELECT
			` + farePricingProfileColumns + `
		FROM fare_pricing_profiles
		WHERE company_id = $1
		  AND ($2 = FALSE OR is_active = TRUE)
		ORDER BY
			service_category_id ASC,
			effective_from DESC,
			created_at DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		companyID,
		activeOnly,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list fare pricing profiles: %w",
			err,
		)
	}
	defer rows.Close()

	profiles :=
		make([]*models.FarePricingProfile, 0)

	for rows.Next() {
		profile, err :=
			scanFarePricingProfile(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan fare pricing profile: %w",
				err,
			)
		}

		profiles =
			append(profiles, profile)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate fare pricing profiles: %w",
			err,
		)
	}

	return profiles, nil
}
