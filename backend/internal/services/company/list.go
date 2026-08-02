package company

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) List(
	ctx context.Context,
) ([]*models.Company, error) {

	return s.companies.List(ctx)
}