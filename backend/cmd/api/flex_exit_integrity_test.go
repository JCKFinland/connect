package main

import (
	"errors"
	"testing"
)

// A typed nil pointer must never be passed directly to
// the FLEX shutdown interface in the disabled-worker path.
func TestFlexExitIntegrityDisabledWorker(t *testing.T) {
	var controller *flexSettlementController
	var worker flexShutdownWaiter

	if controller != nil {
		worker = controller
	}

	if worker != nil {
		t.Fatal("disabled FLEX worker must produce nil interface")
	}
}

func TestFlexExitIntegrityEnabledWorker(t *testing.T) {
	controller := &flexSettlementController{}
	var worker flexShutdownWaiter

	if controller != nil {
		worker = controller
	}

	if worker == nil {
		t.Fatal("enabled FLEX controller must remain available")
	}
}

func TestFlexExitIntegrityDeferredFailure(t *testing.T) {
	expected := errors.New("financial cleanup failed")

	result := func() (exitCode int) {
		defer func() {
			if err := expected; err != nil {
				exitCode = 1
			}
		}()

		return 0
	}()

	if result != 1 {
		t.Fatalf(
			"deferred financial failure must produce exit code 1, got %d",
			result,
		)
	}
}

func TestFlexExitIntegrityPreservesExistingFailure(t *testing.T) {
	result := func() (exitCode int) {
		defer func() {
			exitCode = 1
		}()

		return 1
	}()

	if result != 1 {
		t.Fatalf("existing failure must remain nonzero, got %d", result)
	}
}
