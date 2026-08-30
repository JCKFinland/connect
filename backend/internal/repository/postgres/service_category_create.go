package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/google/uuid"
)

func (r *ServiceCategoryRepository) Create(
	ctx context.Context,
	category *models.ServiceCategory,
) error {
	if category == nil {
		return fmt.Errorf(
			"service category is required",
		)
	}

	if category.ID == "" {
		category.ID = uuid.NewString()
	}

	const query = `
		INSERT INTO service_categories
		(
			id,
			code,
			name,
			description,
			is_active
		)
		VALUES
		(
			$1,$2,$3,$4,$5
		)
		RETURNING
			created_at,
			updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		category.ID,
		category.Code,
		category.Name,
		category.Description,
		category.IsActive,
	).Scan(
		&category.CreatedAt,
		&category.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"create service category: %w",
			err,
		)
	}

	return nil
}
