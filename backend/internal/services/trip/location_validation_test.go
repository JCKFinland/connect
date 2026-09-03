package trip

import (
	"errors"
	"testing"
	"time"
)

func TestValidateRecordLocationRequestAcceptsValidLocation(
	t *testing.T,
) {
	accuracy := 10.0
	speed := 35.0
	altitude := 25.0
	heading := 180

	req := RecordLocationRequest{
		Latitude:       60.1708,
		Longitude:      24.9375,
		Altitude:       &altitude,
		SpeedKMH:       &speed,
		Heading:        &heading,
		AccuracyMeters: &accuracy,
		RecordedAt:     time.Now().UTC(),
	}

	if err := validateRecordLocationRequest(req); err != nil {
		t.Fatalf(
			"expected valid location, got error: %v",
			err,
		)
	}
}

func TestValidateRecordLocationRequestRejectsInvalidCoordinates(
	t *testing.T,
) {
	accuracy := 10.0
	now := time.Now().UTC()

	tests := []struct {
		name      string
		latitude  float64
		longitude float64
	}{
		{
			name:      "latitude below minimum",
			latitude:  -90.1,
			longitude: 24.9375,
		},
		{
			name:      "latitude above maximum",
			latitude:  90.1,
			longitude: 24.9375,
		},
		{
			name:      "longitude below minimum",
			latitude:  60.1708,
			longitude: -180.1,
		},
		{
			name:      "longitude above maximum",
			latitude:  60.1708,
			longitude: 180.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RecordLocationRequest{
				Latitude:       tt.latitude,
				Longitude:      tt.longitude,
				AccuracyMeters: &accuracy,
				RecordedAt:     now,
			}

			err := validateRecordLocationRequest(req)

			if !errors.Is(
				err,
				ErrInvalidTripLocation,
			) {
				t.Fatalf(
					"expected ErrInvalidTripLocation, got %v",
					err,
				)
			}
		})
	}
}

func TestValidateRecordLocationRequestRejectsInvalidAccuracy(
	t *testing.T,
) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		accuracy *float64
	}{
		{
			name:     "missing accuracy",
			accuracy: nil,
		},
		{
			name: "negative accuracy",
			accuracy: func() *float64 {
				value := -1.0
				return &value
			}(),
		},
		{
			name: "accuracy above maximum",
			accuracy: func() *float64 {
				value := maximumLocationAccuracyMeters + 1
				return &value
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RecordLocationRequest{
				Latitude:       60.1708,
				Longitude:      24.9375,
				AccuracyMeters: tt.accuracy,
				RecordedAt:     now,
			}

			err := validateRecordLocationRequest(req)

			if !errors.Is(
				err,
				ErrInvalidTripLocation,
			) {
				t.Fatalf(
					"expected ErrInvalidTripLocation, got %v",
					err,
				)
			}
		})
	}
}

func TestValidateRecordLocationRequestRejectsInvalidSpeedAndHeading(
	t *testing.T,
) {
	accuracy := 10.0
	now := time.Now().UTC()

	t.Run("speed above maximum", func(t *testing.T) {
		speed := maximumLocationSpeedKMH + 1

		req := RecordLocationRequest{
			Latitude:       60.1708,
			Longitude:      24.9375,
			SpeedKMH:       &speed,
			AccuracyMeters: &accuracy,
			RecordedAt:     now,
		}

		err := validateRecordLocationRequest(req)

		if !errors.Is(
			err,
			ErrInvalidTripLocation,
		) {
			t.Fatalf(
				"expected ErrInvalidTripLocation, got %v",
				err,
			)
		}
	})

	t.Run("heading above maximum", func(t *testing.T) {
		heading := 360

		req := RecordLocationRequest{
			Latitude:       60.1708,
			Longitude:      24.9375,
			Heading:        &heading,
			AccuracyMeters: &accuracy,
			RecordedAt:     now,
		}

		err := validateRecordLocationRequest(req)

		if !errors.Is(
			err,
			ErrInvalidTripLocation,
		) {
			t.Fatalf(
				"expected ErrInvalidTripLocation, got %v",
				err,
			)
		}
	})
}

func TestValidateRecordLocationRequestRejectsInvalidTimestamp(
	t *testing.T,
) {
	accuracy := 10.0

	tests := []struct {
		name       string
		recordedAt time.Time
	}{
		{
			name:       "missing timestamp",
			recordedAt: time.Time{},
		},
		{
			name: "stale timestamp",
			recordedAt: time.Now().
				UTC().
				Add(-maximumLocationAge - time.Minute),
		},
		{
			name: "future timestamp",
			recordedAt: time.Now().
				UTC().
				Add(maximumLocationFutureSkew + time.Minute),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RecordLocationRequest{
				Latitude:       60.1708,
				Longitude:      24.9375,
				AccuracyMeters: &accuracy,
				RecordedAt:     tt.recordedAt,
			}

			err := validateRecordLocationRequest(req)

			if !errors.Is(
				err,
				ErrTripLocationTimestamp,
			) {
				t.Fatalf(
					"expected ErrTripLocationTimestamp, got %v",
					err,
				)
			}
		})
	}
}
