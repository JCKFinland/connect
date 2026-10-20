package flex_settlement

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestDiscoveryWorkerCooldownSuppressesImmediateRetry(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	failure := errors.New("provider temporarily unavailable")

	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("settlement-1"),
		},
	}

	dispatch := &workerTestDispatch{
		errByID: map[string]error{
			"settlement-1": failure,
		},
	}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	worker.now = func() time.Time { return now }

	var reported []error
	onError := func(err error) {
		reported = append(reported, err)
	}

	for i := 0; i < 3; i++ {
		if err := worker.RunOnce(context.Background(), onError); err != nil {
			t.Fatal(err)
		}
	}

	if discovery.calls != 3 {
		t.Fatalf("expected three discovery calls, got %d", discovery.calls)
	}

	if len(dispatch.ids) != 1 {
		t.Fatalf("cooldown did not suppress retry: %v", dispatch.ids)
	}

	if len(reported) != 1 || !errors.Is(reported[0], failure) {
		t.Fatalf("unexpected error reports: %v", reported)
	}
}

func TestDiscoveryWorkerCooldownExpires(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("settlement-2"),
		},
	}

	dispatch := &workerTestDispatch{
		errByID: map[string]error{
			"settlement-2": errors.New("temporary failure"),
		},
	}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	worker.now = func() time.Time { return now }

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	now = now.Add(defaultSettlementRetryCooldown - time.Nanosecond)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if len(dispatch.ids) != 1 {
		t.Fatalf("retried before cooldown expired: %v", dispatch.ids)
	}

	now = now.Add(time.Nanosecond)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if len(dispatch.ids) != 2 {
		t.Fatalf("expected retry at expiry, got %v", dispatch.ids)
	}
}

func TestDiscoveryWorkerCooldownIsPerSettlement(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("failing"),
			workerCandidate("healthy"),
		},
	}

	dispatch := &workerTestDispatch{
		errByID: map[string]error{
			"failing": errors.New("provider error"),
		},
	}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	worker.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if err := worker.RunOnce(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}

	expected := []string{"failing", "healthy", "healthy"}

	if len(dispatch.ids) != len(expected) {
		t.Fatalf("unexpected dispatches: %v", dispatch.ids)
	}

	for i, id := range expected {
		if dispatch.ids[i] != id {
			t.Fatalf("got %v, want %v", dispatch.ids, expected)
		}
	}
}

func TestDiscoveryWorkerCooldownClearsAfterSuccess(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	discovery := &workerTestDiscovery{
		candidates: []postgresrepo.FlexSettlementCandidate{
			workerCandidate("recovering"),
		},
	}

	dispatch := &workerTestDispatch{
		errByID: map[string]error{
			"recovering": errors.New("temporary failure"),
		},
	}

	worker, err := NewDiscoveryWorker(discovery, dispatch)
	if err != nil {
		t.Fatal(err)
	}

	worker.now = func() time.Time { return now }

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	delete(dispatch.errByID, "recovering")
	now = now.Add(defaultSettlementRetryCooldown)

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if _, exists := worker.cooldowns["recovering"]; exists {
		t.Fatal("successful settlement retained cooldown")
	}

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	if len(dispatch.ids) != 3 {
		t.Fatalf("successful settlement remained throttled: %v", dispatch.ids)
	}
}

func TestDiscoveryWorkerCooldownMemoryIsBounded(t *testing.T) {
	worker, err := NewDiscoveryWorker(
		&workerTestDiscovery{},
		&workerTestDispatch{},
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	for i := 0; i < maxSettlementCooldownEntries+100; i++ {
		worker.recordCooldown(
			fmt.Sprintf("settlement-%d", i),
			now.Add(time.Duration(i)*time.Second),
		)
	}

	if len(worker.cooldowns) != maxSettlementCooldownEntries {
		t.Fatalf(
			"cooldown map exceeded bound: %d",
			len(worker.cooldowns),
		)
	}

	if _, exists := worker.cooldowns["settlement-0"]; exists {
		t.Fatal("oldest cooldown entry was not evicted")
	}

	newest := fmt.Sprintf(
		"settlement-%d",
		maxSettlementCooldownEntries+99,
	)

	if _, exists := worker.cooldowns[newest]; !exists {
		t.Fatal("newest cooldown entry was not retained")
	}
}
