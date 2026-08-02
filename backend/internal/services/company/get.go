package company

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*models.Company, error) {

	return s.companies.GetByID(
		ctx,
		id,
	)
}
