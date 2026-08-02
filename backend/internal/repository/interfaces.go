package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error

	GetByID(ctx context.Context, id string) (*models.User, error)

	GetByEmail(ctx context.Context, email string) (*models.User, error)

	Update(ctx context.Context, user *models.User) error

	Delete(ctx context.Context, id string) error
}

