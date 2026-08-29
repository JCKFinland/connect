package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

func scanServiceCategory(
	row pgx.Row,
) (*models.ServiceCategory, error) {
	category := &models.ServiceCategory{}

	err := row.Scan(
		&category.ID,
		&category.CompanyID,
		&category.Code,
		&category.Name,
		&category.Description,
		&category.IsActive,
		&category.CreatedAt,
		&category.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return category, nil
}
