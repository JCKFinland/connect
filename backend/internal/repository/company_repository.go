package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type CompanyRepository interface {
	Create(
		ctx context.Context,
		company *models.Company,
	) error

	Update(
		ctx context.Context,
		company *models.Company,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Company, error)

	List(
		ctx context.Context,
	) ([]*models.Company, error)

	Delete(
		ctx context.Context,
		id string,
	) error
}