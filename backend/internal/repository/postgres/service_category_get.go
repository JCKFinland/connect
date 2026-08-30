package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *ServiceCategoryRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.ServiceCategory, error) {
	query := `
		SELECT
			` + serviceCategoryColumns + `
		FROM service_categories
		WHERE id = $1
	`

	category, err := scanServiceCategory(
		r.db.QueryRow(
			ctx,
			query,
			id,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get service category by ID: %w",
			err,
		)
	}

	return category, nil
}

func (r *ServiceCategoryRepository) GetByCode(
	ctx context.Context,
	code string,
) (*models.ServiceCategory, error) {
	query := `
		SELECT
			` + serviceCategoryColumns + `
		FROM service_categories
		WHERE UPPER(code) = UPPER($1)
	`

	category, err := scanServiceCategory(
		r.db.QueryRow(
			ctx,
			query,
			code,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get service category by code: %w",
			err,
		)
	}

	return category, nil
}
