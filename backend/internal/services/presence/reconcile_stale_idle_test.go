package presence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
)

func TestReconcileStaleIdleUsesConfiguredHeartbeatTimeout(t *testing.T) {
	location := time.FixedZone("test", 3*60*60)
	now := time.Date(
		2026,
		time.September,
		30,
		18,
		45,
		30,
		123456000,
		location,
	)

	var (
		called         bool
		gotDriverID    string
		gotStaleBefore time.Time
	)

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireStaleIdle: func(
			ctx context.Context,
			driverID string,
			staleBefore time.Time,
		) (bool, error) {
			called = true
			gotDriverID = driverID
			gotStaleBefore = staleBefore
			return true, nil
		},
	}

	expired, err := service.ReconcileStaleIdle(
		context.Background(),
		"driver-user-id",
		now,
	)
	if err != nil {
		t.Fatalf("reconcile stale idle presence: %v", err)
	}

	if !expired {
		t.Fatal("expected stale presence to be expired")
	}

	if !called {
		t.Fatal("expected stale presence expiration to be called")
	}

	if gotDriverID != "driver-user-id" {
		t.Fatalf(
			"expected driver ID %q, got %q",
			"driver-user-id",
			gotDriverID,
		)
	}

	wantStaleBefore := now.UTC().Add(-2 * time.Minute)

	if !gotStaleBefore.Equal(wantStaleBefore) {
		t.Fatalf(
			"expected stale-before %s, got %s",
			wantStaleBefore,
			gotStaleBefore,
		)
	}

	if gotStaleBefore.Location() != time.UTC {
		t.Fatalf(
			"expected UTC stale-before location, got %s",
			gotStaleBefore.Location(),
		)
	}
}

func TestReconcileStaleIdleReturnsNoTransition(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireStaleIdle: func(
			ctx context.Context,
			driverID string,
			staleBefore time.Time,
		) (bool, error) {
			return false, nil
		},
	}

	expired, err := service.ReconcileStaleIdle(
		context.Background(),
		"driver-user-id",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("reconcile stale idle presence: %v", err)
	}

	if expired {
		t.Fatal("expected presence to remain unchanged")
	}
}

func TestReconcileStaleIdleWrapsRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireStaleIdle: func(
			ctx context.Context,
			driverID string,
			staleBefore time.Time,
		) (bool, error) {
			return false, repositoryErr
		},
	}

	expired, err := service.ReconcileStaleIdle(
		context.Background(),
		"driver-user-id",
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("expected repository error")
	}

	if expired {
		t.Fatal("expected no expiration on repository error")
	}

	if !errors.Is(err, repositoryErr) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"expire stale idle driver presence",
	) {
		t.Fatalf(
			"expected expiration context in error, got %v",
			err,
		)
	}
}

func TestReconcileStaleIdleRejectsInvalidInputsBeforeExpiration(
	t *testing.T,
) {
	validNow := time.Now().UTC()

	tests := []struct {
		name     string
		service  *Service
		driverID string
		now      time.Time
	}{
		{
			name:     "nil service",
			service:  nil,
			driverID: "driver-user-id",
			now:      validNow,
		},
		{
			name: "nil config",
			service: &Service{
				expireStaleIdle: func(
					ctx context.Context,
					driverID string,
					staleBefore time.Time,
				) (bool, error) {
					t.Fatal(
						"expiration must not run with nil config",
					)
					return false, nil
				},
			},
			driverID: "driver-user-id",
			now:      validNow,
		},
		{
			name: "missing expiration dependency",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 2 * time.Minute,
					},
				},
			},
			driverID: "driver-user-id",
			now:      validNow,
		},
		{
			name: "blank driver ID",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 2 * time.Minute,
					},
				},
				expireStaleIdle: func(
					ctx context.Context,
					driverID string,
					staleBefore time.Time,
				) (bool, error) {
					t.Fatal(
						"expiration must not run with blank driver ID",
					)
					return false, nil
				},
			},
			driverID: "",
			now:      validNow,
		},
		{
			name: "zero reconciliation time",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 2 * time.Minute,
					},
				},
				expireStaleIdle: func(
					ctx context.Context,
					driverID string,
					staleBefore time.Time,
				) (bool, error) {
					t.Fatal(
						"expiration must not run with zero time",
					)
					return false, nil
				},
			},
			driverID: "driver-user-id",
			now:      time.Time{},
		},
		{
			name: "non-positive heartbeat timeout",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 0,
					},
				},
				expireStaleIdle: func(
					ctx context.Context,
					driverID string,
					staleBefore time.Time,
				) (bool, error) {
					t.Fatal(
						"expiration must not run with invalid timeout",
					)
					return false, nil
				},
			},
			driverID: "driver-user-id",
			now:      validNow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expired, err := tc.service.ReconcileStaleIdle(
				context.Background(),
				tc.driverID,
				tc.now,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if expired {
				t.Fatal(
					"expected no expiration on validation error",
				)
			}
		})
	}
}
