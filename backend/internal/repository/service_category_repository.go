package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type ServiceCategoryRepository interface {
	Create(
		ctx context.Context,
		category *models.ServiceCategory,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.ServiceCategory, error)

	GetByCode(
		ctx context.Context,
		code string,
	) (*models.ServiceCategory, error)

	List(
		ctx context.Context,
		activeOnly bool,
	) ([]models.ServiceCategory, error)
}
