package presence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestReconcileAllStaleIdleUsesConfiguredHeartbeatTimeout(
	t *testing.T,
) {
	location := time.FixedZone("test", 3*60*60)
	now := time.Date(
		2026,
		time.September,
		30,
		20,
		30,
		45,
		123456000,
		location,
	)

	var (
		called         bool
		gotStaleBefore time.Time
	)

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireAllStaleIdle: func(
			ctx context.Context,
			staleBefore time.Time,
		) (int64, error) {
			called = true
			gotStaleBefore = staleBefore
			return 7, nil
		},
	}

	expiredCount, err := service.ReconcileAllStaleIdle(
		context.Background(),
		now,
	)
	if err != nil {
		t.Fatalf("reconcile all stale idle presence: %v", err)
	}

	if expiredCount != 7 {
		t.Fatalf(
			"expected 7 expired presence rows, got %d",
			expiredCount,
		)
	}

	if !called {
		t.Fatal("expected batch stale presence expiration to be called")
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

func TestReconcileAllStaleIdleWrapsRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")

	service := &Service{
		cfg: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		expireAllStaleIdle: func(
			ctx context.Context,
			staleBefore time.Time,
		) (int64, error) {
			return 0, repositoryErr
		},
	}

	expiredCount, err := service.ReconcileAllStaleIdle(
		context.Background(),
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("expected repository error")
	}

	if expiredCount != 0 {
		t.Fatalf(
			"expected zero expirations on error, got %d",
			expiredCount,
		)
	}

	if !errors.Is(err, repositoryErr) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"expire all stale idle driver presence",
	) {
		t.Fatalf(
			"expected batch expiration context in error, got %v",
			err,
		)
	}
}

func TestReconcileAllStaleIdleRejectsInvalidInputsBeforeExpiration(
	t *testing.T,
) {
	validNow := time.Now().UTC()

	tests := []struct {
		name    string
		service *Service
		now     time.Time
	}{
		{
			name:    "nil service",
			service: nil,
			now:     validNow,
		},
		{
			name: "nil config",
			service: &Service{
				expireAllStaleIdle: func(
					ctx context.Context,
					staleBefore time.Time,
				) (int64, error) {
					t.Fatal(
						"batch expiration must not run with nil config",
					)
					return 0, nil
				},
			},
			now: validNow,
		},
		{
			name: "missing batch expiration dependency",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 2 * time.Minute,
					},
				},
			},
			now: validNow,
		},
		{
			name: "zero reconciliation time",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 2 * time.Minute,
					},
				},
				expireAllStaleIdle: func(
					ctx context.Context,
					staleBefore time.Time,
				) (int64, error) {
					t.Fatal(
						"batch expiration must not run with zero time",
					)
					return 0, nil
				},
			},
			now: time.Time{},
		},
		{
			name: "non-positive heartbeat timeout",
			service: &Service{
				cfg: &config.Config{
					Presence: config.PresenceConfig{
						HeartbeatTimeout: 0,
					},
				},
				expireAllStaleIdle: func(
					ctx context.Context,
					staleBefore time.Time,
				) (int64, error) {
					t.Fatal(
						"batch expiration must not run with invalid timeout",
					)
					return 0, nil
				},
			},
			now: validNow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expiredCount, err :=
				tc.service.ReconcileAllStaleIdle(
					context.Background(),
					tc.now,
				)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if expiredCount != 0 {
				t.Fatalf(
					"expected zero expirations, got %d",
					expiredCount,
				)
			}
		})
	}
}

func TestNewServiceWiresStalePresenceExpiration(t *testing.T) {
	repo := postgres.NewDriverPresenceRepository(nil)

	service := NewService(Dependencies{
		Config: &config.Config{
			Presence: config.PresenceConfig{
				HeartbeatTimeout: 2 * time.Minute,
			},
		},
		Presence: repo,
	})

	if service.expireStaleIdle == nil {
		t.Fatal("expected single stale expiration to be wired")
	}

	if service.expireAllStaleIdle == nil {
		t.Fatal("expected batch stale expiration to be wired")
	}
}
