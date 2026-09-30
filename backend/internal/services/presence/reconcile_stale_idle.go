package presence

import (
	"context"
	"fmt"
	"time"
)

// ReconcileStaleIdle transitions stale idle online presence to OFFLINE.
//
// The service owns heartbeat-timeout policy. The repository performs the
// race-safe mutation and independently protects BUSY and active-trip state.
//
// The returned bool reports whether presence was transitioned to OFFLINE.
func (s *Service) ReconcileStaleIdle(
	ctx context.Context,
	driverID string,
	now time.Time,
) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("presence service is required")
	}

	if s.cfg == nil {
		return false, fmt.Errorf("presence configuration is required")
	}

	if s.expireStaleIdle == nil {
		return false, fmt.Errorf("stale presence expiration is required")
	}

	if driverID == "" {
		return false, fmt.Errorf("driver ID is required")
	}

	if now.IsZero() {
		return false, fmt.Errorf("reconciliation time is required")
	}

	if s.cfg.Presence.HeartbeatTimeout <= 0 {
		return false, fmt.Errorf(
			"presence heartbeat timeout must be greater than zero",
		)
	}

	staleBefore := now.UTC().Add(
		-s.cfg.Presence.HeartbeatTimeout,
	)

	expired, err := s.expireStaleIdle(
		ctx,
		driverID,
		staleBefore,
	)
	if err != nil {
		return false, fmt.Errorf(
			"expire stale idle driver presence: %w",
			err,
		)
	}

	return expired, nil
}
