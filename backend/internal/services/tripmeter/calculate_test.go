package tripmeter

import (
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestCalculateMovingTrip(t *testing.T) {
	base := time.Date(
		2026,
		time.August,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	locations := []*models.TripLocation{
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base,
		},
		{
			Latitude:   60.170500,
			Longitude:  24.940000,
			RecordedAt: base.Add(10 * time.Second),
		},
	}

	measurement, err := Calculate(
		locations,
		DefaultConfig(),
	)
	if err != nil {
		t.Fatalf(
			"calculate trip measurement: %v",
			err,
		)
	}

	if measurement.DistanceMeters < 110 ||
		measurement.DistanceMeters > 120 {
		t.Fatalf(
			"expected distance between 110 and 120 meters, got %d",
			measurement.DistanceMeters,
		)
	}

	if measurement.DurationSeconds != 10 {
		t.Fatalf(
			"expected duration 10 seconds, got %d",
			measurement.DurationSeconds,
		)
	}

	if measurement.WaitingDurationSeconds != 0 {
		t.Fatalf(
			"expected zero waiting duration, got %d",
			measurement.WaitingDurationSeconds,
		)
	}

	if measurement.AcceptedSamples != 2 {
		t.Fatalf(
			"expected 2 accepted samples, got %d",
			measurement.AcceptedSamples,
		)
	}
}

func TestCalculateWaitingTrip(t *testing.T) {
	base := time.Date(
		2026,
		time.August,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	locations := []*models.TripLocation{
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base,
		},
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base.Add(30 * time.Second),
		},
	}

	measurement, err := Calculate(
		locations,
		DefaultConfig(),
	)
	if err != nil {
		t.Fatalf(
			"calculate waiting measurement: %v",
			err,
		)
	}

	if measurement.DistanceMeters != 0 {
		t.Fatalf(
			"expected zero distance, got %d",
			measurement.DistanceMeters,
		)
	}

	if measurement.DurationSeconds != 30 {
		t.Fatalf(
			"expected duration 30 seconds, got %d",
			measurement.DurationSeconds,
		)
	}

	if measurement.WaitingDurationSeconds != 30 {
		t.Fatalf(
			"expected waiting duration 30 seconds, got %d",
			measurement.WaitingDurationSeconds,
		)
	}
}

func TestCalculateRejectsBadAccuracySample(t *testing.T) {
	base := time.Date(
		2026,
		time.August,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	badAccuracy := 100.0

	locations := []*models.TripLocation{
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base,
		},
		{
			Latitude:       60.170000,
			Longitude:      24.939000,
			AccuracyMeters: &badAccuracy,
			RecordedAt:     base.Add(10 * time.Second),
		},
	}

	measurement, err := Calculate(
		locations,
		DefaultConfig(),
	)
	if err != nil {
		t.Fatalf(
			"calculate trip measurement: %v",
			err,
		)
	}

	if measurement.AcceptedSamples != 1 {
		t.Fatalf(
			"expected 1 accepted sample, got %d",
			measurement.AcceptedSamples,
		)
	}

	if measurement.RejectedSamples != 1 {
		t.Fatalf(
			"expected 1 rejected sample, got %d",
			measurement.RejectedSamples,
		)
	}

	if measurement.DistanceMeters != 0 {
		t.Fatalf(
			"expected zero distance, got %d",
			measurement.DistanceMeters,
		)
	}
}

func TestCalculateRejectsImpossibleSpeedSegment(t *testing.T) {
	base := time.Date(
		2026,
		time.August,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	locations := []*models.TripLocation{
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base,
		},
		{
			Latitude:   60.250000,
			Longitude:  25.100000,
			RecordedAt: base.Add(10 * time.Second),
		},
	}

	measurement, err := Calculate(
		locations,
		DefaultConfig(),
	)
	if err != nil {
		t.Fatalf(
			"calculate trip measurement: %v",
			err,
		)
	}

	if measurement.DistanceMeters != 0 {
		t.Fatalf(
			"expected rejected distance to be zero, got %d",
			measurement.DistanceMeters,
		)
	}

	if measurement.DurationSeconds != 0 {
		t.Fatalf(
			"expected rejected duration to be zero, got %d",
			measurement.DurationSeconds,
		)
	}

	if measurement.RejectedSegments != 1 {
		t.Fatalf(
			"expected 1 rejected segment, got %d",
			measurement.RejectedSegments,
		)
	}
}

func TestCalculateRejectsLargeTelemetryGap(t *testing.T) {
	base := time.Date(
		2026,
		time.August,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	locations := []*models.TripLocation{
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base,
		},
		{
			Latitude:   60.170500,
			Longitude:  24.940000,
			RecordedAt: base.Add(5 * time.Minute),
		},
	}

	measurement, err := Calculate(
		locations,
		DefaultConfig(),
	)
	if err != nil {
		t.Fatalf(
			"calculate trip measurement: %v",
			err,
		)
	}

	if measurement.DistanceMeters != 0 {
		t.Fatalf(
			"expected zero distance across large gap, got %d",
			measurement.DistanceMeters,
		)
	}

	if measurement.DurationSeconds != 0 {
		t.Fatalf(
			"expected zero duration across large gap, got %d",
			measurement.DurationSeconds,
		)
	}

	if measurement.RejectedSegments != 1 {
		t.Fatalf(
			"expected 1 rejected segment, got %d",
			measurement.RejectedSegments,
		)
	}
}

func TestCalculateRejectedSampleBreaksMeasurementChain(t *testing.T) {
	base := time.Date(
		2026,
		time.August,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	badAccuracy := 100.0

	locations := []*models.TripLocation{
		{
			Latitude:   60.169856,
			Longitude:  24.938379,
			RecordedAt: base,
		},
		{
			Latitude:       60.170000,
			Longitude:      24.939000,
			AccuracyMeters: &badAccuracy,
			RecordedAt:     base.Add(10 * time.Second),
		},
		{
			Latitude:   60.170500,
			Longitude:  24.940000,
			RecordedAt: base.Add(20 * time.Second),
		},
	}

	measurement, err := Calculate(
		locations,
		DefaultConfig(),
	)
	if err != nil {
		t.Fatalf(
			"calculate trip measurement: %v",
			err,
		)
	}

	if measurement.AcceptedSamples != 2 {
		t.Fatalf(
			"expected 2 accepted samples, got %d",
			measurement.AcceptedSamples,
		)
	}

	if measurement.RejectedSamples != 1 {
		t.Fatalf(
			"expected 1 rejected sample, got %d",
			measurement.RejectedSamples,
		)
	}

	if measurement.DistanceMeters != 0 {
		t.Fatalf(
			"expected rejected middle sample to break distance chain, got %d meters",
			measurement.DistanceMeters,
		)
	}

	if measurement.DurationSeconds != 0 {
		t.Fatalf(
			"expected rejected middle sample to break duration chain, got %d seconds",
			measurement.DurationSeconds,
		)
	}
}
