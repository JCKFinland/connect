package trip

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

type listLocationsTripRepository struct {
	repository.TripRepository

	trip            *models.Trip
	err             error
	requestedTripID string
}

func (r *listLocationsTripRepository) GetByID(
	_ context.Context,
	tripID string,
) (*models.Trip, error) {
	r.requestedTripID = tripID

	if r.err != nil {
		return nil, r.err
	}

	return r.trip, nil
}

type listLocationsUserRoleRepository struct {
	repository.UserRoleRepository

	roles           []string
	err             error
	requestedUserID string
}

func (r *listLocationsUserRoleRepository) GetUserRoles(
	_ context.Context,
	userID string,
) ([]string, error) {
	r.requestedUserID = userID

	if r.err != nil {
		return nil, r.err
	}

	return r.roles, nil
}

type listLocationsRepository struct {
	repository.TripLocationRepository

	locations       []*models.TripLocation
	err             error
	requestedTripID string
	called          bool
}

func (r *listLocationsRepository) ListByTripID(
	_ context.Context,
	tripID string,
) ([]*models.TripLocation, error) {
	r.called = true
	r.requestedTripID = tripID

	if r.err != nil {
		return nil, r.err
	}

	return r.locations, nil
}

func TestListTripLocationsAllowsOwningCustomer(
	t *testing.T,
) {
	const (
		tripID     = "trip-123"
		customerID = "customer-123"
		driverID   = "driver-user-123"
	)

	recordedAt := time.Now().UTC()

	tripRepo := &listLocationsTripRepository{
		trip: &models.Trip{
			BaseModel: models.BaseModel{
				ID: tripID,
			},
			CustomerID: customerID,
			DriverID:   driverID,
		},
	}

	roleRepo := &listLocationsUserRoleRepository{
		roles: []string{"CUSTOMER"},
	}

	locationRepo := &listLocationsRepository{
		locations: []*models.TripLocation{
			{
				TripID:     tripID,
				DriverID:   driverID,
				Latitude:   60.1699,
				Longitude:  24.9384,
				RecordedAt: recordedAt,
			},
		},
	}

	service := NewService(
		Dependencies{
			Trips:         tripRepo,
			UserRoles:     roleRepo,
			TripLocations: locationRepo,
		},
	)

	locations, err := service.ListTripLocations(
		context.Background(),
		tripID,
		customerID,
	)
	if err != nil {
		t.Fatalf(
			"expected customer location access, got error: %v",
			err,
		)
	}

	if len(locations) != 1 {
		t.Fatalf(
			"expected 1 location, got %d",
			len(locations),
		)
	}

	if !locationRepo.called {
		t.Fatal(
			"expected trip locations repository to be called",
		)
	}

	if locationRepo.requestedTripID != tripID {
		t.Fatalf(
			"expected location lookup for %q, got %q",
			tripID,
			locationRepo.requestedTripID,
		)
	}
}

func TestListTripLocationsAllowsAssignedDriver(
	t *testing.T,
) {
	const (
		tripID     = "trip-123"
		customerID = "customer-123"
		driverID   = "driver-user-123"
	)

	tripRepo := &listLocationsTripRepository{
		trip: &models.Trip{
			BaseModel: models.BaseModel{
				ID: tripID,
			},
			CustomerID: customerID,
			DriverID:   driverID,
		},
	}

	roleRepo := &listLocationsUserRoleRepository{
		roles: []string{"DRIVER"},
	}

	locationRepo := &listLocationsRepository{
		locations: []*models.TripLocation{},
	}

	service := NewService(
		Dependencies{
			Trips:         tripRepo,
			UserRoles:     roleRepo,
			TripLocations: locationRepo,
		},
	)

	locations, err := service.ListTripLocations(
		context.Background(),
		tripID,
		driverID,
	)
	if err != nil {
		t.Fatalf(
			"expected assigned driver location access, got error: %v",
			err,
		)
	}

	if locations == nil {
		t.Fatal(
			"expected empty location slice, got nil",
		)
	}

	if len(locations) != 0 {
		t.Fatalf(
			"expected 0 locations, got %d",
			len(locations),
		)
	}
}

func TestListTripLocationsDeniesUnrelatedUser(
	t *testing.T,
) {
	const (
		tripID          = "trip-123"
		customerID      = "customer-owner"
		driverID        = "driver-owner"
		unrelatedUserID = "customer-other"
	)

	tripRepo := &listLocationsTripRepository{
		trip: &models.Trip{
			BaseModel: models.BaseModel{
				ID: tripID,
			},
			CustomerID: customerID,
			DriverID:   driverID,
		},
	}

	roleRepo := &listLocationsUserRoleRepository{
		roles: []string{"CUSTOMER"},
	}

	locationRepo := &listLocationsRepository{}

	service := NewService(
		Dependencies{
			Trips:         tripRepo,
			UserRoles:     roleRepo,
			TripLocations: locationRepo,
		},
	)

	locations, err := service.ListTripLocations(
		context.Background(),
		tripID,
		unrelatedUserID,
	)

	if !errors.Is(
		err,
		ErrTripAccessDenied,
	) {
		t.Fatalf(
			"expected ErrTripAccessDenied, got locations=%v err=%v",
			locations,
			err,
		)
	}

	if locations != nil {
		t.Fatalf(
			"expected nil locations, got %+v",
			locations,
		)
	}

	if locationRepo.called {
		t.Fatal(
			"location repository must not be queried after access denial",
		)
	}
}

func TestListTripLocationsReturnsMissingTrip(
	t *testing.T,
) {
	const (
		tripID = "missing-trip"
		userID = "customer-123"
	)

	tripRepo := &listLocationsTripRepository{
		err: pgx.ErrNoRows,
	}

	roleRepo := &listLocationsUserRoleRepository{
		roles: []string{"CUSTOMER"},
	}

	locationRepo := &listLocationsRepository{}

	service := NewService(
		Dependencies{
			Trips:         tripRepo,
			UserRoles:     roleRepo,
			TripLocations: locationRepo,
		},
	)

	locations, err := service.ListTripLocations(
		context.Background(),
		tripID,
		userID,
	)

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf(
			"expected pgx.ErrNoRows, got locations=%v err=%v",
			locations,
			err,
		)
	}

	if locationRepo.called {
		t.Fatal(
			"location repository must not be queried for missing trip",
		)
	}
}

func TestListTripLocationsNormalizesNilToEmptySlice(
	t *testing.T,
) {
	const (
		tripID     = "trip-123"
		customerID = "customer-123"
	)

	tripRepo := &listLocationsTripRepository{
		trip: &models.Trip{
			BaseModel: models.BaseModel{
				ID: tripID,
			},
			CustomerID: customerID,
		},
	}

	roleRepo := &listLocationsUserRoleRepository{
		roles: []string{"CUSTOMER"},
	}

	locationRepo := &listLocationsRepository{
		locations: nil,
	}

	service := NewService(
		Dependencies{
			Trips:         tripRepo,
			UserRoles:     roleRepo,
			TripLocations: locationRepo,
		},
	)

	locations, err := service.ListTripLocations(
		context.Background(),
		tripID,
		customerID,
	)
	if err != nil {
		t.Fatalf(
			"list trip locations: %v",
			err,
		)
	}

	if locations == nil {
		t.Fatal(
			"expected normalized empty slice, got nil",
		)
	}

	if len(locations) != 0 {
		t.Fatalf(
			"expected empty slice, got %d locations",
			len(locations),
		)
	}
}
