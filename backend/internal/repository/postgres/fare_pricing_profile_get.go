package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *FarePricingProfileRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.FarePricingProfile, error) {
	query := `
		SELECT
			` + farePricingProfileColumns + `
		FROM fare_pricing_profiles
		WHERE id = $1
	`

	profile, err := scanFarePricingProfile(
		r.db.QueryRow(
			ctx,
			query,
			id,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get fare pricing profile by ID: %w",
			err,
		)
	}

	return profile, nil
}

func (r *FarePricingProfileRepository) GetByVersion(
	ctx context.Context,
	companyID string,
	version string,
) (*models.FarePricingProfile, error) {
	query := `
		SELECT
			` + farePricingProfileColumns + `
		FROM fare_pricing_profiles
		WHERE company_id = $1
		  AND version = $2
	`

	profile, err := scanFarePricingProfile(
		r.db.QueryRow(
			ctx,
			query,
			companyID,
			version,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get fare pricing profile by version: %w",
			err,
		)
	}

	return profile, nil
}
