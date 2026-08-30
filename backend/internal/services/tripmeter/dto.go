package tripmeter

// Measurement is the authoritative operational measurement
// calculated from accepted trip location evidence.
type Measurement struct {
	DistanceMeters         int64
	DurationSeconds        int64
	WaitingDurationSeconds int64

	AcceptedSamples  int
	RejectedSamples  int
	RejectedSegments int
}
