package pricing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/services/fare"
)

func (s *service) Resolve(
	ctx context.Context,
	input ResolveInput,
) (*ResolveResult, error) {
	if input.CompanyID == "" {
		return nil, ErrInvalidCompanyID
	}

	if input.ServiceCategoryID == "" {
		return nil, ErrInvalidServiceCategoryID
	}

	if input.At.IsZero() {
		return nil, ErrInvalidEffectiveTime
	}

	profile, err :=
		s.farePricingProfiles.ResolveEffective(
			ctx,
			input.CompanyID,
			input.BranchID,
			input.ServiceCategoryID,
			input.At,
		)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil,
				ErrPricingProfileNotFound
		}

		return nil, fmt.Errorf(
			"resolve effective fare pricing profile: %w",
			err,
		)
	}

	return &ResolveResult{
		ProfileID: profile.ID,

		ServiceCategoryID: profile.ServiceCategoryID,

		Pricing: fare.PricingSnapshot{
			BaseFare: profile.BaseFare,

			DistanceRatePerKM: profile.DistanceRatePerKM,

			TimeRatePerMinute: profile.TimeRatePerMinute,

			WaitingRatePerMinute: profile.WaitingRatePerMinute,

			BookingFee: profile.BookingFee,

			SurgeMultiplier: profile.SurgeMultiplier,

			Currency: profile.Currency,

			PricingVersion: profile.Version,
		},
	}, nil
}
