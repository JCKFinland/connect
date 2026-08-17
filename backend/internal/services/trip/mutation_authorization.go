package trip

import (
	"context"
	"errors"
	"fmt"
)

var ErrTripMutationAccessDenied = errors.New(
	"trip mutation access denied",
)

// authorizeOperationalMutation verifies that the authenticated user
// has an operational role allowed to modify trip administration data.
//
// Generic trip mutation endpoints are intentionally restricted.
// Driver/customer-specific actions should use narrower dedicated
// service methods and endpoints instead of this authorization path.
func (s *tripService) authorizeOperationalMutation(
	ctx context.Context,
	userID string,
) error {

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"get user roles: %w",
			err,
		)
	}

	for _, role := range roles {
		switch role {

		case "SYSTEM_ADMIN",
			"COMPANY_ADMIN",
			"DISPATCHER":

			return nil
		}
	}

	return ErrTripMutationAccessDenied
}
