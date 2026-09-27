package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *DriverEarningRepository) ListRecentByDriverID(
	ctx context.Context,
	driverID string,
	limit int,
) ([]models.DriverEarning, error) {
	const query = `
		SELECT
			` + driverEarningColumns + `
		FROM driver_earnings
		WHERE driver_id = $1
		ORDER BY calculated_at DESC, id DESC
		LIMIT $2
	`

	rows, err := r.db.Query(
		ctx,
		query,
		driverID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list recent driver earnings: %w",
			err,
		)
	}
	defer rows.Close()

	earnings := make([]models.DriverEarning, 0)

	for rows.Next() {
		earning, err := scanDriverEarning(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan recent driver earning: %w",
				err,
			)
		}

		earnings = append(
			earnings,
			*earning,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate recent driver earnings: %w",
			err,
		)
	}

	return earnings, nil
}
