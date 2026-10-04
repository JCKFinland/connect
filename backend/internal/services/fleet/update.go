package fleet

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Update modifies descriptive fields of a fleet within the authenticated
// caller's tenant authority. Tenant, branch, and activation authority are
// immutable through this operation.
func (s *Service) Update(
	ctx context.Context,
	userID string,
	id string,
	req UpdateFleetRequest,
) error {
	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return err
	}

	fleet := &models.Fleet{
		BaseModel: models.BaseModel{
			ID: id,
		},
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
	}

	if systemAdmin {
		err = s.fleets.UpdateDetails(ctx, fleet)
	} else {
		err = s.fleets.UpdateDetailsForCompanyMember(
			ctx,
			userID,
			fleet,
		)
	}

	if errors.Is(err, repository.ErrNotFound) {
		return ErrFleetNotFound
	}

	return err
}
