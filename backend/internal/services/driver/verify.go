package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	var verifiedDriver *models.Driver

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
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

			if driver.Status != "PENDING_VERIFICATION" ||
				driver.IsVerified {
				return ErrDriverNotPendingVerification
			}

			if !driver.IsActive {
				return ErrInvalidDriver
			}

			now := time.Now().UTC()

			if driver.DrivingLicenseExpiry == nil ||
				!driver.DrivingLicenseExpiry.After(now) {
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
