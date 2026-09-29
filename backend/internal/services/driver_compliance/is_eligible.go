package driver_compliance

import (
	"context"
	"time"
)

// IsEligible provides the narrow operational eligibility contract used by
// services such as driver presence and dispatch.
func (s *Service) IsEligible(
	ctx context.Context,
	driverID string,
	now time.Time,
) (bool, error) {
	result, err := s.Evaluate(
		ctx,
		driverID,
		now,
	)
	if err != nil {
		return false, err
	}

	if result == nil {
		return false, nil
	}

	return result.Eligible, nil
}
