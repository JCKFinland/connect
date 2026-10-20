package main

// stopAndWaitBackgroundWorkers ensures operational goroutines
// have actually returned before application resource cleanup.
//
// Both workers are cancelled before either completion is awaited,
// preventing one slow worker from delaying cancellation of the other.
func stopAndWaitBackgroundWorkers(
	cancelRedispatch func(),
	redispatchDone <-chan struct{},
	cancelStalePresence func(),
	stalePresenceDone <-chan struct{},
) {
	cancelRedispatch()
	cancelStalePresence()

	<-redispatchDone
	<-stalePresenceDone
}
