package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type tripLocationAPITestTripRepository struct {
	repository.TripRepository

	result *models.Trip
	err    error
}

func (r *tripLocationAPITestTripRepository) GetByID(
	_ context.Context,
	_ string,
) (*models.Trip, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.result, nil
}

type tripLocationAPITestRoleRepository struct {
	repository.UserRoleRepository

	roles []string
	err   error
}

func (r *tripLocationAPITestRoleRepository) GetUserRoles(
	_ context.Context,
	_ string,
) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.roles, nil
}

type tripLocationAPITestLocationRepository struct {
	repository.TripLocationRepository

	locations []*models.TripLocation
	called    bool
}

func (r *tripLocationAPITestLocationRepository) ListByTripID(
	_ context.Context,
	_ string,
) ([]*models.TripLocation, error) {
	r.called = true
	return r.locations, nil
}

func newTripLocationAPITestContext(
	t *testing.T,
	userID string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/trip-1/locations",
		nil,
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: "trip-1",
		},
	}

	if userID != "" {
		user := &models.User{}
		user.ID = userID

		middleware.SetCurrentUser(
			c,
			user,
		)
	}

	return c, recorder
}

func TestListTripLocationsReturnsLocations(
	t *testing.T,
) {
	const (
		tripID     = "trip-1"
		customerID = "customer-1"
		driverID   = "driver-1"
	)

	recordedAt := time.Now().UTC()

	tripRepo := &tripLocationAPITestTripRepository{
		result: &models.Trip{
			BaseModel: models.BaseModel{
				ID: tripID,
			},
			CustomerID: customerID,
			DriverID:   driverID,
		},
	}

	roleRepo := &tripLocationAPITestRoleRepository{
		roles: []string{"CUSTOMER"},
	}

	locationRepo := &tripLocationAPITestLocationRepository{
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

	service := trip.NewService(
		trip.Dependencies{
			Trips:         tripRepo,
			UserRoles:     roleRepo,
			TripLocations: locationRepo,
		},
	)

	handler := NewTripHandler(service)

	c, recorder := newTripLocationAPITestContext(
		t,
		customerID,
	)

	handler.ListTripLocations(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Success bool                  `json:"success"`
		Data    []models.TripLocation `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if !body.Success {
		t.Fatal("expected success=true")
	}

	if len(body.Data) != 1 {
		t.Fatalf(
			"expected 1 location, got %d",
			len(body.Data),
		)
	}

	if body.Data[0].TripID != tripID {
		t.Fatalf(
			"expected trip ID %q, got %q",
			tripID,
			body.Data[0].TripID,
		)
	}
}

func TestListTripLocationsReturnsEmptyArray(
	t *testing.T,
) {
	const customerID = "customer-1"

	service := trip.NewService(
		trip.Dependencies{
			Trips: &tripLocationAPITestTripRepository{
				result: &models.Trip{
					BaseModel: models.BaseModel{
						ID: "trip-1",
					},
					CustomerID: customerID,
				},
			},
			UserRoles: &tripLocationAPITestRoleRepository{
				roles: []string{"CUSTOMER"},
			},
			TripLocations: &tripLocationAPITestLocationRepository{
				locations: nil,
			},
		},
	)

	handler := NewTripHandler(service)

	c, recorder := newTripLocationAPITestContext(
		t,
		customerID,
	)

	handler.ListTripLocations(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if !body.Success {
		t.Fatal("expected success=true")
	}

	if string(body.Data) != "[]" {
		t.Fatalf(
			"expected data=[], got %s",
			string(body.Data),
		)
	}
}

func TestListTripLocationsReturnsForbiddenForUnrelatedUser(
	t *testing.T,
) {
	const unrelatedUserID = "customer-other"

	locationRepo := &tripLocationAPITestLocationRepository{}

	service := trip.NewService(
		trip.Dependencies{
			Trips: &tripLocationAPITestTripRepository{
				result: &models.Trip{
					BaseModel: models.BaseModel{
						ID: "trip-1",
					},
					CustomerID: "customer-owner",
					DriverID:   "driver-owner",
				},
			},
			UserRoles: &tripLocationAPITestRoleRepository{
				roles: []string{"CUSTOMER"},
			},
			TripLocations: locationRepo,
		},
	)

	handler := NewTripHandler(service)

	c, recorder := newTripLocationAPITestContext(
		t,
		unrelatedUserID,
	)

	handler.ListTripLocations(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusForbidden,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if locationRepo.called {
		t.Fatal(
			"location repository must not be queried after access denial",
		)
	}
}

func TestListTripLocationsReturnsNotFoundForMissingTrip(
	t *testing.T,
) {
	locationRepo := &tripLocationAPITestLocationRepository{}

	service := trip.NewService(
		trip.Dependencies{
			Trips: &tripLocationAPITestTripRepository{
				err: repository.ErrNotFound,
			},
			UserRoles: &tripLocationAPITestRoleRepository{
				roles: []string{"CUSTOMER"},
			},
			TripLocations: locationRepo,
		},
	)

	handler := NewTripHandler(service)

	c, recorder := newTripLocationAPITestContext(
		t,
		"customer-1",
	)

	handler.ListTripLocations(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusNotFound,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if locationRepo.called {
		t.Fatal(
			"location repository must not be queried for missing trip",
		)
	}
}
