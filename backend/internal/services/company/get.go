package company

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) GetByID(
	ctx context.Context,
	userID string,
	id string,
) (*models.Company, error) {
	if s == nil ||
		s.companies == nil ||
		s.userRoles == nil ||
		userID == "" ||
		id == "" {
		return nil, ErrCompanyNotFound
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve company read authority: %w",
			err,
		)
	}

	var company *models.Company

	if systemAdmin {
		company, err = s.companies.GetByID(ctx, id)
	} else {
		company, err = s.companies.GetByIDForCompanyMember(
			ctx,
			userID,
			id,
		)
	}

	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrCompanyNotFound
	}
	if err != nil {
		return nil, err
	}

	return company, nil
}
