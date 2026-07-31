package assignment

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Unassign(
	ctx context.Context,
	req UnassignDriverRequest,
) error {

	_, err := s.assignments.GetActiveByDriver(
		ctx,
		req.DriverID,
	)

	if err == repository.ErrNotFound {
		return ErrAssignmentNotFound
	}

	if err != nil {
		return err
	}

	if err := s.assignments.CloseAssignment(
		ctx,
		req.DriverID,
	); err != nil {
		return err
	}

	return s.presence.DetachAssignment(
		ctx,
		req.DriverID,
	)
}