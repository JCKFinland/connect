package fleet

import "context"

func (s *Service) List(
	ctx context.Context,
) ([]*FleetResponse, error) {

	fleets, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	response := make([]*FleetResponse, 0, len(fleets))

	for _, fleet := range fleets {

		response = append(response, &FleetResponse{
			ID:          fleet.ID,
			CreatedAt:   fleet.CreatedAt,
			UpdatedAt:   fleet.UpdatedAt,
			CompanyID:   fleet.CompanyID,
			BranchID:    fleet.BranchID,
			Code:        fleet.Code,
			Name:        fleet.Name,
			Description: fleet.Description,
			IsActive:    fleet.IsActive,
		})
	}

	return response, nil
}