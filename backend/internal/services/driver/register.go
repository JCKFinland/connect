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

// Register creates an unverified driver profile for the authenticated user.
//
// Registration participates in the company lifecycle lock so the selected
// company and branch cannot cross their lifecycle boundary while the driver
// profile is being created.
//
// Identity and verification state are CONNECT-owned. Self-registration always
// starts at PENDING_VERIFICATION.
func (s *Service) Register(
	ctx context.Context,
	user *models.User,
	req RegisterDriverRequest,
) (*models.Driver, error) {
	if s == nil ||
		user == nil ||
		strings.TrimSpace(user.ID) == "" {
		return nil, ErrInvalidDriver
	}

	req.CompanyID = strings.TrimSpace(req.CompanyID)
	req.BranchID = strings.TrimSpace(req.BranchID)
	req.TaxiDriverLicenseNumber = strings.TrimSpace(
		req.TaxiDriverLicenseNumber,
	)
	req.DrivingLicenseNumber = strings.TrimSpace(
		req.DrivingLicenseNumber,
	)

	if req.CompanyID == "" ||
		req.BranchID == "" ||
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

	if s.db == nil {
		return nil, fmt.Errorf("driver registration database is not configured")
	}

	var created *models.Driver

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"company:"+req.CompanyID,
			); err != nil {
				return fmt.Errorf(
					"lock company lifecycle for driver registration: %w",
					err,
				)
			}

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
			branches := postgresrepo.NewBranchRepositoryWithDB(tx)
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)

			company, err := companies.GetByID(ctx, req.CompanyID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrInvalidDriver
				}

				return fmt.Errorf(
					"get company for driver registration: %w",
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
					"get branch for driver registration: %w",
					err,
				)
			}

			if branch == nil ||
				branch.ID == "" ||
				!branch.IsActive ||
				branch.CompanyID != company.ID {
				return ErrInvalidDriver
			}

			// One active driver profile per platform user. This check is inside
			// the lifecycle transaction; the database constraint remains the
			// final arbiter for concurrent registration attempts.
			_, err = drivers.GetByUserID(ctx, user.ID)
			switch {
			case err == nil:
				return ErrDriverAlreadyExists
			case !errors.Is(err, repository.ErrNotFound):
				return fmt.Errorf(
					"check existing driver registration: %w",
					err,
				)
			}

			now := time.Now().UTC()
			driverID := uuid.NewString()

			driver := &models.Driver{
				BaseModel: models.BaseModel{
					ID:        driverID,
					CreatedAt: now,
					UpdatedAt: now,
				},

				UserID:    user.ID,
				CompanyID: company.ID,
				BranchID:  branch.ID,

				DriverNumber: "DRV-" + strings.ToUpper(
					strings.ReplaceAll(driverID[:12], "-", ""),
				),

				FirstName: strings.TrimSpace(user.FirstName),
				LastName:  strings.TrimSpace(user.LastName),
				Phone:     strings.TrimSpace(user.Phone),
				Email:     strings.TrimSpace(user.Email),

				TaxiDriverLicenseNumber: req.TaxiDriverLicenseNumber,
				DrivingLicenseNumber:    req.DrivingLicenseNumber,
				DrivingLicenseExpiry:    req.DrivingLicenseExpiry,

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
					"create driver registration: %w",
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
