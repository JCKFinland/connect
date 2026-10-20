package main

// shutdownExitCode reports whether the completed shutdown
// should be considered successful by the process supervisor.
//
// Normal OS termination with successful cleanup returns zero.
// Unexpected internal failure or failed cleanup returns one.
func shutdownExitCode(
	decision shutdownDecision,
	shutdownErr error,
) int {
	if decision.Err != nil ||
		decision.Emergency ||
		shutdownErr != nil {
		return 1
	}

	return 0
}
