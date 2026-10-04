package vehicle

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrVehicleCreationAccessDenied = errors.New(
	"vehicle creation access denied",
)

// Create registers a new vehicle through the administrative vehicle surface.
//
// Fleet ownership is the canonical source of company and branch authority.
// Clients cannot choose company_id, branch_id, or initial activation state.
func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateVehicleRequest,
) (*VehicleResponse, error) {
	if userID == "" {
		return nil, ErrVehicleCreationAccessDenied
	}

	if s.fleets == nil {
		return nil, fmt.Errorf("fleet repository is not configured")
	}

	fleet, err := s.fleets.GetByID(ctx, req.FleetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidFleet
		}

		return nil, fmt.Errorf("get fleet: %w", err)
	}

	if fleet == nil ||
		fleet.ID == "" ||
		fleet.CompanyID == "" ||
		fleet.BranchID == "" ||
		!fleet.IsActive {
		return nil, ErrInvalidFleet
	}

	authorized, err := s.canCreateVehicleForCompany(
		ctx,
		userID,
		fleet.CompanyID,
	)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, ErrVehicleCreationAccessDenied
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

	if err := s.vehicles.Create(ctx, vehicle); err != nil {
		return nil, err
	}

	return &VehicleResponse{
		ID:                 vehicle.ID,
		CompanyID:          vehicle.CompanyID,
		BranchID:           vehicle.BranchID,
		FleetID:            vehicle.FleetID,
		RegistrationNumber: vehicle.RegistrationNumber,
		VIN:                vinValue(vehicle.VIN),
		Make:               vehicle.Make,
		Model:              vehicle.Model,
		ModelYear:          vehicle.ModelYear,
		Color:              vehicle.Color,
		VehicleType:        vehicle.VehicleType,
		FuelType:           vehicle.FuelType,
		SeatingCapacity:    vehicle.SeatingCapacity,
		IsActive:           vehicle.IsActive,
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
