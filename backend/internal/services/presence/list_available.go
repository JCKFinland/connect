package presence

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) ListAvailable(
	ctx context.Context,
	companyID string,
) ([]*models.DriverPresence, error) {

	return s.presence.ListAvailable(
		ctx,
		companyID,
	)
}
