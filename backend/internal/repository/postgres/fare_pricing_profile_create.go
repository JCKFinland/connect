package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/google/uuid"
)

func (r *FarePricingProfileRepository) Create(
	ctx context.Context,
	profile *models.FarePricingProfile,
) error {
	if profile == nil {
		return fmt.Errorf(
			"fare pricing profile is required",
		)
	}

	if profile.ID == "" {
		profile.ID = uuid.NewString()
	}

	const query = `
		INSERT INTO fare_pricing_profiles
		(
			id,
			company_id,
			branch_id,
			service_category_id,
			version,
			currency,
			base_fare,
			distance_rate_per_km,
			time_rate_per_minute,
			waiting_rate_per_minute,
			booking_fee,
			surge_multiplier,
			effective_from,
			effective_to,
			is_active,
			created_by_user_id
		)
		VALUES
		(
			$1,$2,$3,$4,$5,$6,$7,$8,
			$9,$10,$11,$12,$13,$14,$15,$16
		)
		RETURNING created_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		profile.ID,
		profile.CompanyID,
		profile.BranchID,
		profile.ServiceCategoryID,
		profile.Version,
		profile.Currency,
		profile.BaseFare,
		profile.DistanceRatePerKM,
		profile.TimeRatePerMinute,
		profile.WaitingRatePerMinute,
		profile.BookingFee,
		profile.SurgeMultiplier,
		profile.EffectiveFrom,
		profile.EffectiveTo,
		profile.IsActive,
		profile.CreatedByUserID,
	).Scan(
		&profile.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"create fare pricing profile: %w",
			err,
		)
	}

	return nil
}
