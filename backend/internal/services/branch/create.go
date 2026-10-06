package branch

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
	ErrBranchCreationAccessDenied = errors.New("branch creation access denied")
	ErrInvalidCompany             = errors.New("invalid or inactive company")
)

// Create registers a new active branch.
//
// Company ownership is the canonical source of tenant authority. Creation
// participates in the company lifecycle lock so an active branch cannot race
// company deactivation or archival.
func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateBranchRequest,
) (*models.Branch, error) {
	if s == nil ||
		s.db == nil ||
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

	var created *models.Branch

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"company:"+req.CompanyID,
			); err != nil {
				return fmt.Errorf(
					"lock company lifecycle for branch creation: %w",
					err,
				)
			}

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
			branches := postgresrepo.NewBranchRepositoryWithDB(tx)

			var company *models.Company
			var err error

			if systemAdmin {
				company, err = companies.GetByID(
					ctx,
					req.CompanyID,
				)
			} else {
				company, err = companies.GetByIDForCompanyMember(
					ctx,
					userID,
					req.CompanyID,
				)
			}

			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					if systemAdmin {
						return ErrInvalidCompany
					}
					return ErrBranchCreationAccessDenied
				}
				return fmt.Errorf("get company: %w", err)
			}

			if company == nil ||
				company.ID == "" ||
				!company.IsActive {
				return ErrInvalidCompany
			}

			branch := &models.Branch{
				CompanyID:    company.ID,
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

			if err := branches.Create(ctx, branch); err != nil {
				return err
			}

			created = branch
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return created, nil
}
