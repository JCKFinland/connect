package tripmeter

// Config defines the rules used to validate GPS evidence and
// calculate authoritative trip measurements.
type Config struct {
	// MaximumAccuracyMeters rejects GPS samples whose reported
	// horizontal accuracy is worse than this value.
	MaximumAccuracyMeters float64

	// MaximumSpeedKMH rejects movement segments that would imply
	// an impossible or unacceptable vehicle speed.
	MaximumSpeedKMH float64

	// WaitingSpeedThresholdKMH defines the maximum movement speed
	// considered stationary/waiting.
	WaitingSpeedThresholdKMH float64

	// MaximumSampleGapSeconds prevents large telemetry gaps from
	// being treated as continuously observed movement or waiting.
	MaximumSampleGapSeconds int64
}

// DefaultConfig returns the initial CONNECT trip-meter rules.
//
// These values are operational defaults and can later become
// centrally configurable once the meter policy is finalized.
func DefaultConfig() Config {
	return Config{
		MaximumAccuracyMeters:    50,
		MaximumSpeedKMH:          180,
		WaitingSpeedThresholdKMH: 3,
		MaximumSampleGapSeconds:  120,
	}
}
