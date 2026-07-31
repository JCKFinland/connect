package assignment

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) Current(
	ctx context.Context,
	driverID string,
) (*models.DriverAssignment, error) {

	return s.assignments.GetActiveByDriver(
		ctx,
		driverID,
	)
}