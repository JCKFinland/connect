package presence

import (
	"context"
	"fmt"
	"time"
)

// ReconcileAllStaleIdle transitions all stale idle online presence to OFFLINE.
//
// The service owns heartbeat-timeout policy. The repository performs the
// atomic batch mutation and independently protects BUSY and active-trip state.
//
// The returned count is the number of presence rows transitioned to OFFLINE.
func (s *Service) ReconcileAllStaleIdle(
	ctx context.Context,
	now time.Time,
) (int64, error) {
	if s == nil {
		return 0, fmt.Errorf("presence service is required")
	}

	if s.cfg == nil {
		return 0, fmt.Errorf("presence configuration is required")
	}

	if s.expireAllStaleIdle == nil {
		return 0, fmt.Errorf(
			"batch stale presence expiration is required",
		)
	}

	if now.IsZero() {
		return 0, fmt.Errorf("reconciliation time is required")
	}

	if s.cfg.Presence.HeartbeatTimeout <= 0 {
		return 0, fmt.Errorf(
			"presence heartbeat timeout must be greater than zero",
		)
	}

	staleBefore := now.UTC().Add(
		-s.cfg.Presence.HeartbeatTimeout,
	)

	expiredCount, err := s.expireAllStaleIdle(
		ctx,
		staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"expire all stale idle driver presence: %w",
			err,
		)
	}

	return expiredCount, nil
}
