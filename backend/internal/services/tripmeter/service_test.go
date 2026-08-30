package tripmeter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type tripLocationRepositoryStub struct {
	locations []*models.TripLocation
	err       error
}

func (r *tripLocationRepositoryStub) Create(
	ctx context.Context,
	location *models.TripLocation,
) error {
	return nil
}

func (r *tripLocationRepositoryStub) ListByTripID(
	ctx context.Context,
	tripID string,
) ([]*models.TripLocation, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.locations, nil
}

func TestServiceMeasureTrip(t *testing.T) {
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

	repo := &tripLocationRepositoryStub{
		locations: []*models.TripLocation{
			{
				TripID:     "trip-1",
				Latitude:   60.169856,
				Longitude:  24.938379,
				RecordedAt: base,
			},
			{
				TripID:     "trip-1",
				Latitude:   60.170500,
				Longitude:  24.940000,
				RecordedAt: base.Add(10 * time.Second),
			},
		},
	}

	service := NewService(
		Dependencies{
			TripLocations: repo,
		},
	)

	measurement, err := service.MeasureTrip(
		context.Background(),
		"trip-1",
	)
	if err != nil {
		t.Fatalf(
			"measure trip: %v",
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
}

func TestServiceMeasureTripRequiresTripID(t *testing.T) {
	service := NewService(
		Dependencies{
			TripLocations: &tripLocationRepositoryStub{},
		},
	)

	_, err := service.MeasureTrip(
		context.Background(),
		"",
	)

	if err == nil {
		t.Fatal("expected error for missing trip id")
	}
}

func TestServiceMeasureTripPropagatesRepositoryError(t *testing.T) {
	expectedErr := errors.New("database unavailable")

	service := NewService(
		Dependencies{
			TripLocations: &tripLocationRepositoryStub{
				err: expectedErr,
			},
		},
	)

	_, err := service.MeasureTrip(
		context.Background(),
		"trip-1",
	)

	if err == nil {
		t.Fatal("expected repository error")
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected wrapped repository error %v, got %v",
			expectedErr,
			err,
		)
	}
}
