package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCKFinland/connect/backend/internal/dateutil"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Verify approves a pending driver application and grants the DRIVER role.
// The driver transition, verification audit metadata, and role assignment
// are committed atomically in one PostgreSQL transaction.
func (s *Service) Verify(
	ctx context.Context,
	driverID string,
	verifiedByUserID string,
) (*models.Driver, error) {
	driverID = strings.TrimSpace(driverID)
	verifiedByUserID = strings.TrimSpace(verifiedByUserID)

	if s == nil || s.db == nil || driverID == "" || verifiedByUserID == "" {
		return nil, ErrInvalidDriver
	}

	// Pre-read only to establish deterministic company -> branch -> driver
	// lifecycle lock ordering. The driver is authoritatively re-read with
	// FOR UPDATE after the parent lifecycle locks are held.
	lockTargetRepo := postgresrepo.NewDriverRepository(s.db)

	lockTarget, err := lockTargetRepo.GetByID(ctx, driverID)
	if errors.Is(err, pgx.ErrNoRows) ||
		errors.Is(err, repository.ErrNotFound) {
		return nil, ErrDriverNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get driver verification lock target: %w", err)
	}
	if lockTarget == nil ||
		lockTarget.CompanyID == "" ||
		lockTarget.BranchID == "" {
		return nil, ErrInvalidDriver
	}

	companyID := lockTarget.CompanyID
	branchID := lockTarget.BranchID

	var verifiedDriver *models.Driver

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"company:"+companyID,
			); err != nil {
				return fmt.Errorf(
					"lock company lifecycle for driver verification: %w",
					err,
				)
			}

			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"branch:"+branchID,
			); err != nil {
				return fmt.Errorf(
					"lock branch lifecycle for driver verification: %w",
					err,
				)
			}

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
			branches := postgresrepo.NewBranchRepositoryWithDB(tx)
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
			roles := repository.NewRoleRepository(tx)
			userRoles := repository.NewUserRoleRepository(tx)

			driver, err := drivers.GetByIDForUpdate(ctx, driverID)
			if errors.Is(err, pgx.ErrNoRows) ||
				errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf("get driver for verification: %w", err)
			}

			// If ownership changed after the pre-read, the lifecycle locks above
			// protect the wrong parents. Fail closed rather than activating the
			// driver under stale lock targets.
			if driver.CompanyID != companyID ||
				driver.BranchID != branchID {
				return ErrInvalidDriver
			}

			company, err := companies.GetByID(ctx, companyID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrInvalidDriver
			}
			if err != nil {
				return fmt.Errorf(
					"get company for driver verification: %w",
					err,
				)
			}
			if company == nil || !company.IsActive {
				return ErrInvalidDriver
			}

			branch, err := branches.GetByID(ctx, branchID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrInvalidDriver
			}
			if err != nil {
				return fmt.Errorf(
					"get branch for driver verification: %w",
					err,
				)
			}
			if branch == nil ||
				!branch.IsActive ||
				branch.CompanyID != companyID {
				return ErrInvalidDriver
			}

			if driver.Status != "PENDING_VERIFICATION" ||
				driver.IsVerified {
				return ErrDriverNotPendingVerification
			}

			if !driver.IsActive {
				return ErrInvalidDriver
			}

			now := time.Now().UTC()

			if driver.DrivingLicenseExpiry == nil ||
				dateutil.Before(*driver.DrivingLicenseExpiry, now) {
				return ErrInvalidDriver
			}

			driverRole, err := roles.GetByName(ctx, "DRIVER")
			if err != nil {
				return fmt.Errorf("get DRIVER role: %w", err)
			}

			driver.Status = "ACTIVE"
			driver.IsVerified = true
			driver.VerifiedAt = &now
			driver.VerifiedByUserID = &verifiedByUserID

			if err := drivers.Update(ctx, driver); err != nil {
				return fmt.Errorf("update verified driver: %w", err)
			}

			err = userRoles.AssignRole(
				ctx,
				driver.UserID,
				driverRole.ID,
			)
			if err != nil &&
				!errors.Is(err, repository.ErrRoleAlreadyAssigned) {
				return fmt.Errorf("assign DRIVER role: %w", err)
			}

			verifiedDriver = driver

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return verifiedDriver, nil
}
