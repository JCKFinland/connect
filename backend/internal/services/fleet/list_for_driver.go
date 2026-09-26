package fleet

import (
	"context"
	"errors"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) ListForDriver(
	ctx context.Context,
	userID string,
) ([]*FleetResponse, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrDriverNotEligible
	}

	driver, err := s.drivers.GetByUserID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrDriverNotEligible
	}
	if err != nil {
		return nil, err
	}

	if driver.Status != "ACTIVE" ||
		!driver.IsVerified ||
		!driver.IsActive {
		return nil, ErrDriverNotEligible
	}

	fleets, err := s.fleets.ListActiveByCompanyAndBranch(
		ctx,
		driver.CompanyID,
		driver.BranchID,
	)
	if err != nil {
		return nil, err
	}

	response := make([]*FleetResponse, 0, len(fleets))

	for _, item := range fleets {
		response = append(response, &FleetResponse{
			ID:          item.ID,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
			CompanyID:   item.CompanyID,
			BranchID:    item.BranchID,
			Code:        item.Code,
			Name:        item.Name,
			Description: item.Description,
			IsActive:    item.IsActive,
		})
	}

	return response, nil
}
