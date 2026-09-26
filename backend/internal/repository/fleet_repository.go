package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type FleetRepository interface {
	Create(
		ctx context.Context,
		fleet *models.Fleet,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Fleet, error)

	List(
		ctx context.Context,
	) ([]*models.Fleet, error)

	ListActiveByCompanyAndBranch(
		ctx context.Context,
		companyID string,
		branchID string,
	) ([]*models.Fleet, error)

	Update(
		ctx context.Context,
		fleet *models.Fleet,
	) error

	Delete(
		ctx context.Context,
		id string,
	) error
}
