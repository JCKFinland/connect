package vehicle

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrVehicleCreationAccessDenied = errors.New(
	"vehicle creation access denied",
)

// Create registers a new vehicle through the administrative vehicle surface.
//
// Fleet ownership is the canonical source of company and branch authority.
// Clients cannot choose company_id, branch_id, or initial activation state.
//
// Creation shares the owning fleet's transaction-scoped advisory lock with
// fleet lifecycle operations. This prevents a vehicle from being inserted
// from stale fleet state while the fleet is being archived or deactivated.
func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateVehicleRequest,
) (*VehicleResponse, error) {
	if s == nil ||
		s.db == nil ||
		s.userRoles == nil ||
		userID == "" ||
		req.FleetID == "" {

		return nil, ErrVehicleCreationAccessDenied
	}

	var created *models.Vehicle

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"fleet:"+req.FleetID,
			); err != nil {
				return fmt.Errorf(
					"lock fleet lifecycle for vehicle creation: %w",
					err,
				)
			}

			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)
			vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)

			// Re-read the fleet only after acquiring its lifecycle lock.
			// GetByID excludes archived fleets, so archived and nonexistent
			// parents are both invalid creation targets.
			fleet, err := fleets.GetByID(
				ctx,
				req.FleetID,
			)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrInvalidFleet
				}

				return fmt.Errorf("get fleet: %w", err)
			}

			if fleet == nil ||
				fleet.ID == "" ||
				fleet.CompanyID == "" ||
				fleet.BranchID == "" ||
				!fleet.IsActive {

				return ErrInvalidFleet
			}

			authorized, err := s.canCreateVehicleForCompany(
				ctx,
				userID,
				fleet.CompanyID,
			)
			if err != nil {
				return err
			}
			if !authorized {
				return ErrVehicleCreationAccessDenied
			}

			vehicle := &models.Vehicle{
				CompanyID:          fleet.CompanyID,
				BranchID:           fleet.BranchID,
				FleetID:            fleet.ID,
				RegistrationNumber: req.RegistrationNumber,
				VIN:                normalizeVIN(req.VIN),
				Make:               req.Make,
				Model:              req.Model,
				ModelYear:          req.ModelYear,
				Color:              req.Color,
				VehicleType:        req.VehicleType,
				FuelType:           req.FuelType,
				SeatingCapacity:    req.SeatingCapacity,
				IsActive:           true,
			}

			if err := vehicles.Create(
				ctx,
				vehicle,
			); err != nil {
				return err
			}

			created = vehicle
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &VehicleResponse{
		ID:                 created.ID,
		CompanyID:          created.CompanyID,
		BranchID:           created.BranchID,
		FleetID:            created.FleetID,
		RegistrationNumber: created.RegistrationNumber,
		VIN:                vinValue(created.VIN),
		Make:               created.Make,
		Model:              created.Model,
		ModelYear:          created.ModelYear,
		Color:              created.Color,
		VehicleType:        created.VehicleType,
		FuelType:           created.FuelType,
		SeatingCapacity:    created.SeatingCapacity,
		IsActive:           created.IsActive,
	}, nil
}

func (s *Service) canCreateVehicleForCompany(
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
