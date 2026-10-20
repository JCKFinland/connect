package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const processExitHelperEnvironment = "CONNECT_PROCESS_EXIT_HELPER"

// TestProcessExitWrapper verifies that process exit happens
// after the application execution function has returned.
//
// The subprocess uses only a test helper. It never starts
// CONNECT, connects to PostgreSQL, or runs migrations.
func TestProcessExitWrapper(t *testing.T) {
	if os.Getenv(processExitHelperEnvironment) == "1" {
		code := testApplicationExecution()
		os.Exit(code)
	}

	command := exec.Command(
		os.Args[0],
		"-test.run=^TestProcessExitWrapper$",
	)

	command.Env = append(
		os.Environ(),
		processExitHelperEnvironment+"=1",
	)

	output, err := command.CombinedOutput()

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf(
			"expected subprocess exit error, got %v; output=%s",
			err,
			output,
		)
	}

	if exitError.ExitCode() != 1 {
		t.Fatalf(
			"expected exit code 1, got %d; output=%s",
			exitError.ExitCode(),
			output,
		)
	}

	if !strings.Contains(
		string(output),
		"CONNECT_TEST_DEFER_EXECUTED",
	) {
		t.Fatalf(
			"deferred cleanup did not execute before process exit: %s",
			output,
		)
	}
}

func testApplicationExecution() int {
	defer func() {
		println("CONNECT_TEST_DEFER_EXECUTED")
	}()

	return 1
}
