package vehicle

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Update modifies descriptive fields of a vehicle within the authenticated
// caller's tenant authority. Tenant, fleet, and activation authority are
// immutable through this operation.
func (s *Service) Update(
	ctx context.Context,
	userID string,
	id string,
	req UpdateVehicleRequest,
) error {
	if userID == "" {
		return ErrVehicleCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return err
	}

	vehicle := &models.Vehicle{
		BaseModel: models.BaseModel{
			ID: id,
		},
		RegistrationNumber: req.RegistrationNumber,
		VIN:                normalizeVIN(req.VIN),
		Make:               req.Make,
		Model:              req.Model,
		ModelYear:          req.ModelYear,
		Color:              req.Color,
		VehicleType:        req.VehicleType,
		FuelType:           req.FuelType,
		SeatingCapacity:    req.SeatingCapacity,
	}

	if systemAdmin {
		return s.vehicles.UpdateDetails(
			ctx,
			vehicle,
		)
	}

	return s.vehicles.UpdateDetailsForCompanyMember(
		ctx,
		userID,
		vehicle,
	)
}
