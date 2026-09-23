package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) ListRegistrationCompanies(
	ctx context.Context,
) ([]*models.Company, error) {
	if s.companies == nil {
		return nil, fmt.Errorf("company repository is not configured")
	}

	companies, err := s.companies.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list registration companies: %w", err)
	}

	return companies, nil
}

func (s *Service) ListRegistrationBranches(
	ctx context.Context,
	companyID string,
) ([]*models.Branch, error) {
	companyID = strings.TrimSpace(companyID)
	if companyID == "" {
		return nil, ErrInvalidDriver
	}

	if s.companies == nil {
		return nil, fmt.Errorf("company repository is not configured")
	}

	if s.branches == nil {
		return nil, fmt.Errorf("branch repository is not configured")
	}

	company, err := s.companies.GetByID(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("get registration company: %w", err)
	}

	if !company.IsActive {
		return nil, ErrInvalidDriver
	}

	branches, err := s.branches.ListActiveByCompanyID(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list registration branches: %w", err)
	}

	return branches, nil
}
