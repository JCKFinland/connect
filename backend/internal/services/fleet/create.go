package fleet

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var (
	ErrFleetCreationAccessDenied = errors.New("fleet creation access denied")
	ErrInvalidBranch             = errors.New("invalid or inactive branch")
)

// Create registers a new active fleet through the administrative fleet surface.
//
// Branch ownership is the canonical source of company authority. Creation
// participates in the branch lifecycle lock so an active fleet cannot race
// branch deactivation or archival.
func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateFleetRequest,
) (*FleetResponse, error) {
	if s == nil ||
		s.db == nil ||
		s.userRoles == nil ||
		userID == "" ||
		req.BranchID == "" {
		return nil, ErrFleetCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve fleet creation authority: %w",
			err,
		)
	}

	var created *models.Fleet

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"branch:"+req.BranchID,
			); err != nil {
				return fmt.Errorf(
					"lock branch lifecycle for fleet creation: %w",
					err,
				)
			}

			branches := postgresrepo.NewBranchRepositoryWithDB(tx)
			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)

			var branch *models.Branch
			var err error

			if systemAdmin {
				branch, err = branches.GetByID(
					ctx,
					req.BranchID,
				)
			} else {
				branch, err = branches.GetByIDForCompanyMember(
					ctx,
					userID,
					req.BranchID,
				)
			}

			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrInvalidBranch
				}
				return fmt.Errorf("get branch: %w", err)
			}

			if branch == nil ||
				branch.ID == "" ||
				branch.CompanyID == "" ||
				!branch.IsActive {
				return ErrInvalidBranch
			}

			fleet := &models.Fleet{
				CompanyID:   branch.CompanyID,
				BranchID:    branch.ID,
				Code:        req.Code,
				Name:        req.Name,
				Description: req.Description,
				IsActive:    true,
			}

			if err := fleets.Create(ctx, fleet); err != nil {
				return err
			}

			created = fleet
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &FleetResponse{
		ID:          created.ID,
		CreatedAt:   created.CreatedAt,
		UpdatedAt:   created.UpdatedAt,
		CompanyID:   created.CompanyID,
		BranchID:    created.BranchID,
		Code:        created.Code,
		Name:        created.Name,
		Description: created.Description,
		IsActive:    created.IsActive,
	}, nil
}
