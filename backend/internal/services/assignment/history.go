package assignment

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) History(
	ctx context.Context,
	driverID string,
) ([]*models.DriverAssignment, error) {

	return s.assignments.ListByDriver(
		ctx,
		driverID,
	)
}