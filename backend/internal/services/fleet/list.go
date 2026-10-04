package fleet

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) List(
	ctx context.Context,
	userID string,
) ([]*FleetResponse, error) {
	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}

	var fleets []*models.Fleet

	if systemAdmin {
		fleets, err = s.fleets.List(ctx)
	} else {
		fleets, err = s.fleets.ListForCompanyMember(
			ctx,
			userID,
		)
	}
	if err != nil {
		return nil, err
	}

	response := make([]*FleetResponse, 0, len(fleets))

	for _, fleet := range fleets {
		response = append(response, fleetResponse(fleet))
	}

	return response, nil
}
