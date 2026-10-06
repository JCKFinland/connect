package branch

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var ErrBranchCreationAccessDenied = fmt.Errorf("branch creation access denied")

func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateBranchRequest,
) (*models.Branch, error) {
	if s == nil ||
		s.branches == nil ||
		s.userRoles == nil ||
		userID == "" ||
		req.CompanyID == "" {
		return nil, ErrBranchCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve branch creation authority: %w",
			err,
		)
	}

	if !systemAdmin {
		if s.companyMemberships == nil {
			return nil, ErrBranchCreationAccessDenied
		}

		member, err := s.companyMemberships.Exists(
			ctx,
			userID,
			req.CompanyID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"check branch company membership: %w",
				err,
			)
		}
		if !member {
			return nil, ErrBranchCreationAccessDenied
		}
	}

	branch := &models.Branch{
		CompanyID:    req.CompanyID,
		Code:         req.Code,
		Name:         req.Name,
		Email:        req.Email,
		Phone:        req.Phone,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		State:        req.State,
		PostalCode:   req.PostalCode,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		IsActive:     true,
	}

	if err := s.branches.Create(ctx, branch); err != nil {
		return nil, err
	}

	return branch, nil
}
