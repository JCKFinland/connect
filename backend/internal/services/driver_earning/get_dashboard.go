package driver_earning

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) GetDashboard(
	ctx context.Context,
	userID string,
) (*models.DriverEarningsDashboard, error) {
	summary, err :=
		s.earnings.GetSummaryByDriverID(
			ctx,
			userID,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"get driver earnings summary: %w",
			err,
		)
	}

	recent, err :=
		s.earnings.ListRecentByDriverID(
			ctx,
			userID,
			recentEarningsLimit,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"list recent driver earnings: %w",
			err,
		)
	}

	return &models.DriverEarningsDashboard{
		Summary:        *summary,
		RecentEarnings: recent,
	}, nil
}
