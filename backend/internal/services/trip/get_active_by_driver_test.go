package trip

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type activeDriverTripRepository struct {
	repository.TripRepository

	trip              *models.Trip
	err               error
	requestedDriverID string
	called            bool
}

func (r *activeDriverTripRepository) GetActiveByDriverID(
	_ context.Context,
	driverID string,
) (*models.Trip, error) {
	r.called = true
	r.requestedDriverID = driverID

	if r.err != nil {
		return nil, r.err
	}

	return r.trip, nil
}

type activeDriverTripUserRoleRepository struct {
	repository.UserRoleRepository

	roles           []string
	err             error
	requestedUserID string
}

func (r *activeDriverTripUserRoleRepository) GetUserRoles(
	_ context.Context,
	userID string,
) ([]string, error) {
	r.requestedUserID = userID

	if r.err != nil {
		return nil, r.err
	}

	return r.roles, nil
}

func TestGetActiveByDriverReturnsCurrentTrip(t *testing.T) {
	const driverUserID = "driver-user-123"

	tripRepo := &activeDriverTripRepository{
		trip: &models.Trip{
			BaseModel: models.BaseModel{
				ID: "trip-123",
			},
			DriverID: driverUserID,
			Status:   StatusAssigned,
		},
	}

	roleRepo := &activeDriverTripUserRoleRepository{
		roles: []string{"DRIVER"},
	}

	service := NewService(Dependencies{
		Trips:     tripRepo,
		UserRoles: roleRepo,
	})

	result, err := service.GetActiveByDriver(
		context.Background(),
		driverUserID,
	)
	if err != nil {
		t.Fatalf("expected active trip, got error: %v", err)
	}

	if result == nil || result.ID != "trip-123" {
		t.Fatalf("expected trip-123, got %#v", result)
	}

	if !tripRepo.called {
		t.Fatal("expected trip repository to be called")
	}

	if tripRepo.requestedDriverID != driverUserID {
		t.Fatalf(
			"expected driver lookup for %q, got %q",
			driverUserID,
			tripRepo.requestedDriverID,
		)
	}

	if roleRepo.requestedUserID != driverUserID {
		t.Fatalf(
			"expected role lookup for %q, got %q",
			driverUserID,
			roleRepo.requestedUserID,
		)
	}
}

func TestGetActiveByDriverRejectsNonDriver(t *testing.T) {
	const userID = "customer-123"

	tripRepo := &activeDriverTripRepository{}

	roleRepo := &activeDriverTripUserRoleRepository{
		roles: []string{"CUSTOMER"},
	}

	service := NewService(Dependencies{
		Trips:     tripRepo,
		UserRoles: roleRepo,
	})

	_, err := service.GetActiveByDriver(
		context.Background(),
		userID,
	)
	if !errors.Is(err, ErrTripAccessDenied) {
		t.Fatalf(
			"expected ErrTripAccessDenied, got %v",
			err,
		)
	}

	if tripRepo.called {
		t.Fatal("trip repository must not be called for non-driver")
	}
}

func TestGetActiveByDriverReturnsNotFound(t *testing.T) {
	const driverUserID = "driver-user-123"

	tripRepo := &activeDriverTripRepository{
		err: repository.ErrNotFound,
	}

	roleRepo := &activeDriverTripUserRoleRepository{
		roles: []string{"DRIVER"},
	}

	service := NewService(Dependencies{
		Trips:     tripRepo,
		UserRoles: roleRepo,
	})

	_, err := service.GetActiveByDriver(
		context.Background(),
		driverUserID,
	)
	if !errors.Is(err, ErrActiveTripNotFound) {
		t.Fatalf(
			"expected ErrActiveTripNotFound, got %v",
			err,
		)
	}
}
