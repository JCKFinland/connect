package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type BranchRepository interface {
	Create(
		ctx context.Context,
		branch *models.Branch,
	) error

	Update(
		ctx context.Context,
		branch *models.Branch,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Branch, error)

	List(
		ctx context.Context,
	) ([]*models.Branch, error)

	Delete(
		ctx context.Context,
		id string,
	) error
}