package vehicle

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID returns a vehicle visible to the authenticated user.
func (s *Service) GetByID(
	ctx context.Context,
	userID string,
	id string,
) (*VehicleResponse, error) {
	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}

	var vehicle *models.Vehicle

	if systemAdmin {
		vehicle, err = s.vehicles.GetByID(ctx, id)
	} else {
		vehicle, err = s.vehicles.GetByIDForCompanyMember(
			ctx,
			userID,
			id,
		)
	}
	if err != nil {
		return nil, err
	}

	return vehicleResponse(vehicle), nil
}

func (s *Service) isSystemAdmin(
	ctx context.Context,
	userID string,
) (bool, error) {
	if userID == "" {
		return false, ErrVehicleCreationAccessDenied
	}
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

	return false, nil
}

func vehicleResponse(vehicle *models.Vehicle) *VehicleResponse {
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
	}
}
