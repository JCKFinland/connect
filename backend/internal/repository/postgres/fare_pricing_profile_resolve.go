package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *FarePricingProfileRepository) ResolveEffective(
	ctx context.Context,
	companyID string,
	branchID *string,
	serviceCategoryID string,
	at time.Time,
) (*models.FarePricingProfile, error) {
	query := `
		SELECT
			` + farePricingProfileColumns + `
		FROM fare_pricing_profiles
		WHERE company_id = $1
		  AND service_category_id = $2
		  AND is_active = TRUE
		  AND effective_from <= $3
		  AND (
				effective_to IS NULL
				OR $3 < effective_to
		  )
		  AND (
				branch_id = $4
				OR branch_id IS NULL
		  )
		ORDER BY
			CASE
				WHEN branch_id = $4 THEN 0
				ELSE 1
			END,
			effective_from DESC,
			created_at DESC
		LIMIT 1
	`

	profile, err := scanFarePricingProfile(
		r.db.QueryRow(
			ctx,
			query,
			companyID,
			serviceCategoryID,
			at,
			branchID,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve effective fare pricing profile: %w",
			err,
		)
	}

	return profile, nil
}
