package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFlexSettlementWorkerDependenciesRejectMissingKey(
	t *testing.T,
) {
	for _, key := range []string{"", "   "} {
		dependencies, err := newFlexSettlementWorkerDependencies(key)

		if err == nil {
			t.Fatalf("missing Stripe key %q accepted", key)
		}

		if dependencies != nil {
			t.Fatal("dependencies returned on invalid configuration")
		}
	}
}

func TestFlexSettlementWorkerDependenciesConstruct(
	t *testing.T,
) {
	dependencies, err := newFlexSettlementWorkerDependencies(
		"sk_test_factory_configuration_only",
	)
	if err != nil {
		t.Fatal(err)
	}

	if dependencies == nil ||
		dependencies.Verifier == nil ||
		dependencies.Capture == nil {
		t.Fatal("incomplete FLEX worker dependencies")
	}
}

// TestFlexSettlementWorkerBuildsInactiveLifecycle verifies that
// complete dependencies produce a lifecycle without starting
// background financial processing.
func TestFlexSettlementWorkerBuildsInactiveLifecycle(
	t *testing.T,
) {
	dependencies, err := newFlexSettlementWorkerDependencies(
		"sk_test_factory_configuration_only",
	)
	if err != nil {
		t.Fatalf("construct Stripe dependencies: %v", err)
	}

	// The factory only retains the pool. No database operations
	// occur until the worker is explicitly started.
	db := &pgxpool.Pool{}

	lifecycle, err := buildFlexSettlementWorker(
		db,
		dependencies,
		10,
	)
	if err != nil {
		t.Fatalf("construct FLEX worker lifecycle: %v", err)
	}

	if lifecycle == nil {
		t.Fatal("factory returned nil lifecycle")
	}

	// Stop before Start must be safe and must not start work.
	lifecycle.Stop()

	// An inactive lifecycle cannot be waited on as if it ran.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	if err := lifecycle.Wait(ctx); err == nil {
		t.Fatal("inactive lifecycle unexpectedly reported completion")
	}
}

func TestFlexSettlementWorkerRejectsMissingDatabase(
	t *testing.T,
) {
	dependencies, err := newFlexSettlementWorkerDependencies(
		"sk_test_factory_configuration_only",
	)
	if err != nil {
		t.Fatal(err)
	}

	lifecycle, err := buildFlexSettlementWorker(
		nil,
		dependencies,
		10,
	)

	if err == nil || lifecycle != nil {
		t.Fatal("worker factory accepted missing database")
	}
}

func TestFlexSettlementWorkerRejectsMissingDependencies(
	t *testing.T,
) {
	db := &pgxpool.Pool{}

	cases := []struct {
		name string
		deps *flexSettlementWorkerDependencies
	}{
		{"nil", nil},
		{"empty", &flexSettlementWorkerDependencies{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle, err := buildFlexSettlementWorker(
				db,
				tc.deps,
				10,
			)

			if err == nil || lifecycle != nil {
				t.Fatal("worker factory accepted missing dependencies")
			}
		})
	}
}

func TestFlexSettlementWorkerRejectsMissingCapture(
	t *testing.T,
) {
	dependencies, err := newFlexSettlementWorkerDependencies(
		"sk_test_factory_configuration_only",
	)
	if err != nil {
		t.Fatal(err)
	}

	dependencies.Capture = nil

	lifecycle, err := buildFlexSettlementWorker(
		&pgxpool.Pool{},
		dependencies,
		10,
	)

	if err == nil || lifecycle != nil {
		t.Fatal("worker factory accepted missing capture executor")
	}
}

func TestFlexSettlementWorkerRejectsMissingVerifier(
	t *testing.T,
) {
	dependencies, err := newFlexSettlementWorkerDependencies(
		"sk_test_factory_configuration_only",
	)
	if err != nil {
		t.Fatal(err)
	}

	dependencies.Verifier = nil

	lifecycle, err := buildFlexSettlementWorker(
		&pgxpool.Pool{},
		dependencies,
		10,
	)

	if err == nil || lifecycle != nil {
		t.Fatal("worker factory accepted missing verifier")
	}
}
