package presence

import (
	"context"
	"errors"
)

var ErrDriverAssignmentRequired = errors.New(
	"driver assignment required before going online",
)

func (s *Service) GoOnline(
	ctx context.Context,
	req GoOnlineRequest,
) error {

	/*
		This method will be fully implemented after
		the Driver Assignment repository/service is introduced.

		A driver cannot go ONLINE until CONNECT knows:

		- Company
		- Branch
		- Vehicle
		- Assignment

		This prevents orphaned presence records and ensures
		the Dispatch Engine only works with valid driver assignments.
	*/

	return ErrDriverAssignmentRequired
}