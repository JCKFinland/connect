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
		companyID string,
		code string,
	) (*models.ServiceCategory, error)

	ListByCompanyID(
		ctx context.Context,
		companyID string,
		activeOnly bool,
	) ([]*models.ServiceCategory, error)
}
