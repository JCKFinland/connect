package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

func scanFarePricingProfile(
	row pgx.Row,
) (*models.FarePricingProfile, error) {
	profile := &models.FarePricingProfile{}

	err := row.Scan(
		&profile.ID,
		&profile.CompanyID,
		&profile.BranchID,
		&profile.ServiceCategoryID,
		&profile.Version,
		&profile.Currency,
		&profile.BaseFare,
		&profile.DistanceRatePerKM,
		&profile.TimeRatePerMinute,
		&profile.WaitingRatePerMinute,
		&profile.BookingFee,
		&profile.SurgeMultiplier,
		&profile.EffectiveFrom,
		&profile.EffectiveTo,
		&profile.IsActive,
		&profile.CreatedByUserID,
		&profile.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return profile, nil
}
