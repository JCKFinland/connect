package tripmeter

import (
	"fmt"
	"math"
	"sort"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Calculate derives authoritative trip measurements from raw GPS evidence.
//
// The input slice is copied and ordered by RecordedAt so callers do not need
// to guarantee ordering and the original slice is not mutated.
func Calculate(
	locations []*models.TripLocation,
	cfg Config,
) (Measurement, error) {

	if cfg.MaximumAccuracyMeters <= 0 {
		return Measurement{}, fmt.Errorf(
			"maximum accuracy meters must be greater than zero",
		)
	}

	if cfg.MaximumSpeedKMH <= 0 {
		return Measurement{}, fmt.Errorf(
			"maximum speed km/h must be greater than zero",
		)
	}

	if cfg.WaitingSpeedThresholdKMH < 0 {
		return Measurement{}, fmt.Errorf(
			"waiting speed threshold km/h cannot be negative",
		)
	}

	if cfg.MaximumSampleGapSeconds <= 0 {
		return Measurement{}, fmt.Errorf(
			"maximum sample gap seconds must be greater than zero",
		)
	}

	ordered := make([]*models.TripLocation, 0, len(locations))

	for _, location := range locations {
		if location == nil {
			continue
		}

		ordered = append(
			ordered,
			location,
		)
	}

	sort.SliceStable(
		ordered,
		func(i int, j int) bool {
			return ordered[i].RecordedAt.Before(
				ordered[j].RecordedAt,
			)
		},
	)

	var (
		measurement    Measurement
		previous       *models.TripLocation
		distanceMeters float64
	)

	for _, current := range ordered {
		if !isUsableLocation(current, cfg) {
			measurement.RejectedSamples++

			// A rejected point breaks continuity. We must not
			// bridge movement across untrusted GPS evidence.
			previous = nil

			continue
		}

		measurement.AcceptedSamples++

		if previous == nil {
			previous = current
			continue
		}

		deltaSeconds := int64(
			current.RecordedAt.
				Sub(previous.RecordedAt).
				Seconds(),
		)

		if deltaSeconds <= 0 {
			measurement.RejectedSegments++

			// Start a fresh chain from the current valid sample.
			previous = current
			continue
		}

		if deltaSeconds > cfg.MaximumSampleGapSeconds {
			measurement.RejectedSegments++

			// The current point is valid, but continuity across
			// the telemetry gap is not authoritative.
			previous = current
			continue
		}

		segmentDistance := haversineDistanceMeters(
			previous.Latitude,
			previous.Longitude,
			current.Latitude,
			current.Longitude,
		)

		if math.IsNaN(segmentDistance) ||
			math.IsInf(segmentDistance, 0) ||
			segmentDistance < 0 {
			measurement.RejectedSegments++
			previous = current
			continue
		}

		impliedSpeedKMH :=
			(segmentDistance / float64(deltaSeconds)) * 3.6

		if impliedSpeedKMH > cfg.MaximumSpeedKMH {
			measurement.RejectedSegments++

			// Do not bridge through an implausible movement
			// segment. Establish a new chain at current.
			previous = current
			continue
		}

		distanceMeters += segmentDistance
		measurement.DurationSeconds += deltaSeconds

		if impliedSpeedKMH <= cfg.WaitingSpeedThresholdKMH {
			measurement.WaitingDurationSeconds += deltaSeconds
		}

		previous = current
	}

	measurement.DistanceMeters = int64(
		math.Round(distanceMeters),
	)

	return measurement, nil
}

func isUsableLocation(
	location *models.TripLocation,
	cfg Config,
) bool {

	if location.RecordedAt.IsZero() {
		return false
	}

	if location.Latitude < -90 ||
		location.Latitude > 90 {
		return false
	}

	if location.Longitude < -180 ||
		location.Longitude > 180 {
		return false
	}

	if location.AccuracyMeters != nil &&
		*location.AccuracyMeters > cfg.MaximumAccuracyMeters {
		return false
	}

	return true
}
