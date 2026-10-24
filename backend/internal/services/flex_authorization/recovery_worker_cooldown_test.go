package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestAuthorizationRecoveryCooldownSuppressesImmediateRetry(
	t *testing.T,
) {
	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates("failed", "healthy"),
	}

	executor := &authorizationWorkerExecutorFake{
		errByID: map[string]error{
			"failed": errors.New("temporary provider error"),
		},
	}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 2,
	)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if err := worker.RunOnce(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}

	executor.mu.Lock()
	defer executor.mu.Unlock()

	var failedCount, healthyCount int

	for _, id := range executor.ids {
		switch id {
		case "failed":
			failedCount++
		case "healthy":
			healthyCount++
		}
	}

	if failedCount != 1 {
		t.Fatalf("failed authorization retried %d times", failedCount)
	}

	if healthyCount != 2 {
		t.Fatalf("healthy authorization processed %d times", healthyCount)
	}
}

func TestAuthorizationRecoveryCooldownExpires(t *testing.T) {
	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates("failed"),
	}

	executor := &authorizationWorkerExecutorFake{
		errByID: map[string]error{
			"failed": errors.New("temporary failure"),
		},
	}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 1,
	)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }

	run := func() {
		t.Helper()
		if err := worker.RunOnce(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}

	run()

	now = now.Add(defaultAuthorizationRecoveryRetryCooldown - time.Nanosecond)
	run()

	if len(executor.ids) != 1 {
		t.Fatal("authorization retried before cooldown expired")
	}

	now = now.Add(time.Nanosecond)
	run()

	if len(executor.ids) != 2 {
		t.Fatal("authorization not retried at cooldown deadline")
	}
}

func TestAuthorizationRecoveryCooldownClearsAfterSuccess(
	t *testing.T,
) {
	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates("authorization"),
	}

	executor := &authorizationWorkerExecutorFake{
		errByID: map[string]error{
			"authorization": errors.New("temporary failure"),
		},
	}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 1,
	)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }

	run := func() {
		t.Helper()
		if err := worker.RunOnce(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}

	run()

	if len(worker.cooldowns) != 1 {
		t.Fatal("failure cooldown not recorded")
	}

	delete(executor.errByID, "authorization")

	now = now.Add(defaultAuthorizationRecoveryRetryCooldown)
	run()

	if len(worker.cooldowns) != 0 {
		t.Fatal("successful recovery did not clear cooldown")
	}

	run()

	if len(executor.ids) != 3 {
		t.Fatal("successful authorization remained throttled")
	}
}

func TestAuthorizationRecoveryCooldownBoundedEviction(
	t *testing.T,
) {
	discovery := &authorizationWorkerDiscoveryFake{}
	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 1,
	)

	base := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	for i := 0; i < maxAuthorizationRecoveryCooldownEntries; i++ {
		worker.recordCooldown(
			fmt.Sprintf("authorization-%04d", i),
			base.Add(time.Duration(i)*time.Second),
		)
	}

	if len(worker.cooldowns) !=
		maxAuthorizationRecoveryCooldownEntries {
		t.Fatalf("unexpected initial cooldown count: %d",
			len(worker.cooldowns))
	}

	worker.recordCooldown(
		"new-authorization",
		base.Add(24*time.Hour),
	)

	if len(worker.cooldowns) !=
		maxAuthorizationRecoveryCooldownEntries {
		t.Fatalf("cooldown map exceeded limit: %d",
			len(worker.cooldowns))
	}

	if _, exists := worker.cooldowns["authorization-0000"]; exists {
		t.Fatal("oldest cooldown was not evicted")
	}

	if _, exists := worker.cooldowns["new-authorization"]; !exists {
		t.Fatal("new cooldown was not retained")
	}
}

func TestAuthorizationRecoveryCooldownUpdatesExistingEntry(
	t *testing.T,
) {
	discovery := &authorizationWorkerDiscoveryFake{}
	executor := &authorizationWorkerExecutorFake{}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 1,
	)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	worker.recordCooldown("authorization", now)
	worker.recordCooldown("authorization", now.Add(time.Hour))

	if len(worker.cooldowns) != 1 {
		t.Fatal("duplicate cooldown entry created")
	}

	if !worker.cooldowns["authorization"].Equal(now.Add(time.Hour)) {
		t.Fatal("existing cooldown deadline not updated")
	}
}

func TestAuthorizationRecoveryCooldownRecordsWithoutErrorCallback(
	t *testing.T,
) {
	discovery := &authorizationWorkerDiscoveryFake{
		candidates: authorizationWorkerCandidates("failed"),
	}

	executor := &authorizationWorkerExecutorFake{
		errByID: map[string]error{
			"failed": errors.New("provider unavailable"),
		},
	}

	worker := newAuthorizationWorkerForTest(
		t, discovery, executor, 1,
	)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if len(worker.cooldowns) != 1 {
		t.Fatal("cooldown missing when error callback is nil")
	}
}
