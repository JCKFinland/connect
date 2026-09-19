package presence

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) GetCurrent(
	ctx context.Context,
	userID string,
) (*PresenceResponse, error) {

	driver, err := s.getDriverByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	current, err := s.presence.GetByDriverID(ctx, driver.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return &PresenceResponse{
			DriverID:           driver.UserID,
			IsOnline:           false,
			AvailabilityStatus: "OFFLINE",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return &PresenceResponse{
		DriverID:           current.DriverID,
		IsOnline:           current.IsOnline,
		AvailabilityStatus: current.AvailabilityStatus,
		LastHeartbeatAt:    current.LastHeartbeatAt,
	}, nil
}
