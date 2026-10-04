package vehicle

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns vehicles visible to the authenticated user.
func (s *Service) List(
	ctx context.Context,
	userID string,
) ([]VehicleResponse, error) {
	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}

	var vehicles []models.Vehicle

	if systemAdmin {
		vehicles, err = s.vehicles.List(ctx)
	} else {
		vehicles, err = s.vehicles.ListForCompanyMember(ctx, userID)
	}
	if err != nil {
		return nil, err
	}

	response := make([]VehicleResponse, 0, len(vehicles))
	for i := range vehicles {
		response = append(response, *vehicleResponse(&vehicles[i]))
	}

	return response, nil
}
