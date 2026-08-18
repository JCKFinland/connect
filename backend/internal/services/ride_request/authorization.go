package ride_request

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var ErrRideRequestAccessDenied = errors.New(
	"ride request access denied",
)

func (s *Service) getUserRoles(
	ctx context.Context,
	userID string,
) ([]string, error) {

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get user roles: %w",
			err,
		)
	}

	return roles, nil
}

func isPrivilegedRideRequestRole(
	roles []string,
) bool {

	for _, role := range roles {
		switch role {

		case "SYSTEM_ADMIN",
			"COMPANY_ADMIN",
			"DISPATCHER":
			return true
		}
	}

	return false
}

func canViewRideRequest(
	roles []string,
	userID string,
	request *models.RideRequest,
) bool {

	if request == nil || userID == "" {
		return false
	}

	if isPrivilegedRideRequestRole(roles) {
		return true
	}

	return request.CustomerID == userID
}
