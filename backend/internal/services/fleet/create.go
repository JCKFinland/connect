package fleet

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

var (
	ErrFleetCreationAccessDenied = errors.New("fleet creation access denied")
	ErrInvalidBranch             = errors.New("invalid or inactive branch")
)

// Create registers a new fleet through the administrative fleet surface.
//
// Branch ownership is the canonical source of company authority.
// Clients cannot choose company_id or initial activation state.
func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateFleetRequest,
) (*FleetResponse, error) {
	if userID == "" {
		return nil, ErrFleetCreationAccessDenied
	}

	if s.branches == nil {
		return nil, fmt.Errorf("branch repository is not configured")
	}

	branch, err := s.branches.GetByID(ctx, req.BranchID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidBranch
		}

		return nil, fmt.Errorf("get branch: %w", err)
	}

	if branch == nil ||
		branch.ID == "" ||
		branch.CompanyID == "" ||
		!branch.IsActive {
		return nil, ErrInvalidBranch
	}

	authorized, err := s.canCreateFleetForCompany(
		ctx,
		userID,
		branch.CompanyID,
	)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, ErrFleetCreationAccessDenied
	}

	fleet := &models.Fleet{
		CompanyID:   branch.CompanyID,
		BranchID:    branch.ID,
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    true,
	}

	if err := s.fleets.Create(ctx, fleet); err != nil {
		return nil, err
	}

	return &FleetResponse{
		ID:          fleet.ID,
		CreatedAt:   fleet.CreatedAt,
		UpdatedAt:   fleet.UpdatedAt,
		CompanyID:   fleet.CompanyID,
		BranchID:    fleet.BranchID,
		Code:        fleet.Code,
		Name:        fleet.Name,
		Description: fleet.Description,
		IsActive:    fleet.IsActive,
	}, nil
}

func (s *Service) canCreateFleetForCompany(
	ctx context.Context,
	userID string,
	companyID string,
) (bool, error) {
	if s.userRoles == nil {
		return false, fmt.Errorf("user role repository is not configured")
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user roles: %w", err)
	}

	for _, role := range roles {
		if role == "SYSTEM_ADMIN" {
			return true, nil
		}
	}

	if s.companyMemberships == nil {
		return false, fmt.Errorf(
			"company membership repository is not configured",
		)
	}

	member, err := s.companyMemberships.Exists(
		ctx,
		userID,
		companyID,
	)
	if err != nil {
		return false, fmt.Errorf("check company membership: %w", err)
	}

	return member, nil
}
