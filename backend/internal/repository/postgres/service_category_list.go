package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *ServiceCategoryRepository) ListByCompanyID(
	ctx context.Context,
	companyID string,
	activeOnly bool,
) ([]*models.ServiceCategory, error) {
	query := `
		SELECT
			` + serviceCategoryColumns + `
		FROM service_categories
		WHERE company_id = $1
		  AND ($2 = FALSE OR is_active = TRUE)
		ORDER BY name ASC, id ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		companyID,
		activeOnly,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list service categories: %w",
			err,
		)
	}
	defer rows.Close()

	categories :=
		make([]*models.ServiceCategory, 0)

	for rows.Next() {
		category, err :=
			scanServiceCategory(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan service category: %w",
				err,
			)
		}

		categories =
			append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate service categories: %w",
			err,
		)
	}

	return categories, nil
}
