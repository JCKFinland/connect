package presence

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrDriverAssignmentRequired = errors.New(
	"driver assignment required before going online",
)

func (s *Service) GoOnline(
	ctx context.Context,
	req GoOnlineRequest,
) error {

	assignment, err := s.assignments.GetActiveByDriver(
		ctx,
		req.DriverID,
	)

	if err == repository.ErrNotFound {
		return ErrDriverAssignmentRequired
	}

	if err != nil {
		return err
	}

	if err := s.AttachAssignment(
		ctx,
		assignment,
	); err != nil {
		return err
	}

	return s.presence.UpdateAvailability(
		ctx,
		req.DriverID,
		StatusAvailable,
		true,
	)
}