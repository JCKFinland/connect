package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/dateutil"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// Create administratively creates an unverified driver profile.
//
// SYSTEM_ADMIN may create a driver in any active company. Other callers must
// have explicit membership in the target company. Creation participates in the
// company lifecycle lock so a driver cannot be created while its tenant is
// concurrently being deactivated or archived.
//
// Verification and operational status are CONNECT-owned lifecycle state.
// Administrative creation always starts at PENDING_VERIFICATION.
func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateDriverRequest,
) (*models.Driver, error) {
	if s == nil ||
		s.db == nil ||
		s.userRoles == nil ||
		strings.TrimSpace(userID) == "" {
		return nil, ErrDriverCreationAccessDenied
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.CompanyID = strings.TrimSpace(req.CompanyID)
	req.BranchID = strings.TrimSpace(req.BranchID)
	req.DriverNumber = strings.TrimSpace(req.DriverNumber)
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Email = strings.TrimSpace(req.Email)
	req.TaxiDriverLicenseNumber = strings.TrimSpace(
		req.TaxiDriverLicenseNumber,
	)
	req.DrivingLicenseNumber = strings.TrimSpace(
		req.DrivingLicenseNumber,
	)

	if req.UserID == "" ||
		req.CompanyID == "" ||
		req.BranchID == "" ||
		req.DriverNumber == "" ||
		req.FirstName == "" ||
		req.LastName == "" ||
		req.Phone == "" ||
		req.TaxiDriverLicenseNumber == "" ||
		req.DrivingLicenseNumber == "" ||
		req.DrivingLicenseExpiry == nil {
		return nil, ErrInvalidDriver
	}

	if dateutil.Before(
		*req.DrivingLicenseExpiry,
		time.Now().UTC(),
	) {
		return nil, ErrInvalidDriver
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve driver creation authority: %w",
			err,
		)
	}

	systemAdmin := false
	for _, role := range roles {
		if role == "SYSTEM_ADMIN" {
			systemAdmin = true
			break
		}
	}

	var created *models.Driver

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
					"lock company lifecycle for driver creation: %w",
					err,
				)
			}

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
			branches := postgresrepo.NewBranchRepositoryWithDB(tx)
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)

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
						return ErrInvalidDriver
					}
					return ErrDriverCreationAccessDenied
				}

				return fmt.Errorf(
					"get company for driver creation: %w",
					err,
				)
			}

			if company == nil ||
				company.ID == "" ||
				!company.IsActive {
				return ErrInvalidDriver
			}

			branch, err := branches.GetByID(ctx, req.BranchID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrInvalidDriver
				}

				return fmt.Errorf(
					"get branch for driver creation: %w",
					err,
				)
			}

			if branch == nil ||
				branch.ID == "" ||
				!branch.IsActive ||
				branch.CompanyID != company.ID {
				return ErrInvalidDriver
			}

			now := time.Now().UTC()

			driver := &models.Driver{
				BaseModel: models.BaseModel{
					ID:        uuid.NewString(),
					CreatedAt: now,
					UpdatedAt: now,
				},

				UserID:       req.UserID,
				CompanyID:    company.ID,
				BranchID:     branch.ID,
				DriverNumber: req.DriverNumber,

				FirstName: req.FirstName,
				LastName:  req.LastName,
				Phone:     req.Phone,
				Email:     req.Email,

				TaxiDriverLicenseNumber: req.TaxiDriverLicenseNumber,
				DrivingLicenseNumber:    req.DrivingLicenseNumber,
				DrivingLicenseExpiry:    req.DrivingLicenseExpiry,

				HireDate: req.HireDate,

				Status:     "PENDING_VERIFICATION",
				IsVerified: false,
				IsActive:   true,
			}

			if err := drivers.Create(ctx, driver); err != nil {
				var pgErr *pgconn.PgError

				if errors.As(err, &pgErr) &&
					pgErr.Code == "23505" &&
					pgErr.ConstraintName == "idx_drivers_unique_active_user" {
					return ErrDriverAlreadyExists
				}

				return fmt.Errorf(
					"create administrative driver: %w",
					err,
				)
			}

			created = driver
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return created, nil
}
