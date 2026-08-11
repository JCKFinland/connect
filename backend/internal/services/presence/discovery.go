package presence

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) ListAvailableForUser(
	ctx context.Context,
	userID string,
) ([]*models.DriverPresence, error) {

	driver, err := s.getDriverByUserID(
		ctx,
		userID,
	)
	if err != nil {
		return nil, err
	}

	return s.ListAvailable(
		ctx,
		driver.CompanyID,
	)
}
