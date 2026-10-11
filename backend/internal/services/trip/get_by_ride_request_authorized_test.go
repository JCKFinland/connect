package trip

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/jackc/pgx/v5"
)

type discoveryAuthorizer struct {
	request *models.RideRequest
	err     error
	called  bool
}

func (a *discoveryAuthorizer) GetByIDAuthorized(
	_ context.Context, _, _ string,
) (*models.RideRequest, error) {
	a.called = true
	if a.err != nil {
		return nil, a.err
	}
	return a.request, nil
}

type discoveryTripRepository struct {
	repository.TripRepository
	tripID      string
	trip        *models.Trip
	lookupErr   error
	lookupCalls int
	getCalls    int
}

func (r *discoveryTripRepository) GetIDByRideRequestID(
	_ context.Context, _ string,
) (string, error) {
	r.lookupCalls++
	return r.tripID, r.lookupErr
}

func (r *discoveryTripRepository) GetByID(
	_ context.Context, _ string,
) (*models.Trip, error) {
	r.getCalls++
	return r.trip, nil
}

type discoveryUserRoles struct {
	repository.UserRoleRepository
}

func (r *discoveryUserRoles) GetUserRoles(
	_ context.Context, _ string,
) ([]string, error) {
	return []string{"CUSTOMER"}, nil
}

func newDiscoveryService(
	authorizer *discoveryAuthorizer,
	trips *discoveryTripRepository,
) Service {
	return NewService(Dependencies{
		Trips:             trips,
		RideRequestAccess: authorizer,
		UserRoles:         &discoveryUserRoles{},
	})
}

func TestTripDiscoveryReturnsOwnedTrip(t *testing.T) {
	const (
		requestID  = "request-123"
		customerID = "customer-123"
		tripID     = "trip-123"
	)

	authorizer := &discoveryAuthorizer{
		request: &models.RideRequest{
			BaseModel:  models.BaseModel{ID: requestID},
			CustomerID: customerID,
		},
	}

	trips := &discoveryTripRepository{
		tripID: tripID,
		trip: &models.Trip{
			BaseModel:     models.BaseModel{ID: tripID},
			RideRequestID: requestID,
			CustomerID:    customerID,
		},
	}

	service := newDiscoveryService(authorizer, trips)

	result, err := service.GetByRideRequestIDAuthorized(
		context.Background(), requestID, customerID,
	)
	if err != nil {
		t.Fatalf("expected owned trip, got %v", err)
	}
	if result == nil || result.ID != tripID {
		t.Fatalf("unexpected trip: %#v", result)
	}
	if !authorizer.called || trips.lookupCalls != 1 || trips.getCalls != 1 {
		t.Fatalf("unexpected calls: authorizer=%v lookup=%d get=%d",
			authorizer.called, trips.lookupCalls, trips.getCalls)
	}
}

func TestTripDiscoveryRejectsUnauthorizedRideRequest(t *testing.T) {
	authorizer := &discoveryAuthorizer{
		err: ride_request.ErrRideRequestAccessDenied,
	}
	trips := &discoveryTripRepository{}
	service := newDiscoveryService(authorizer, trips)

	result, err := service.GetByRideRequestIDAuthorized(
		context.Background(), "request-123", "other-customer",
	)
	if !errors.Is(err, ride_request.ErrRideRequestAccessDenied) {
		t.Fatalf("expected ride-request access denial, got %v", err)
	}
	if result != nil || trips.lookupCalls != 0 || trips.getCalls != 0 {
		t.Fatalf("unauthorized lookup executed: result=%v lookup=%d get=%d",
			result, trips.lookupCalls, trips.getCalls)
	}
}

func TestTripDiscoveryReturnsNoTripYet(t *testing.T) {
	authorizer := &discoveryAuthorizer{
		request: &models.RideRequest{
			BaseModel:  models.BaseModel{ID: "request-123"},
			CustomerID: "customer-123",
		},
	}
	trips := &discoveryTripRepository{
		lookupErr: pgx.ErrNoRows,
	}
	service := newDiscoveryService(authorizer, trips)

	result, err := service.GetByRideRequestIDAuthorized(
		context.Background(), "request-123", "customer-123",
	)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected no trip yet, got %v", err)
	}
	if result != nil || trips.lookupCalls != 1 || trips.getCalls != 0 {
		t.Fatalf("unexpected lookup behavior: result=%v lookup=%d get=%d",
			result, trips.lookupCalls, trips.getCalls)
	}
}

func TestTripDiscoveryRejectsUnauthorizedTrip(t *testing.T) {
	authorizer := &discoveryAuthorizer{
		request: &models.RideRequest{
			BaseModel:  models.BaseModel{ID: "request-123"},
			CustomerID: "customer-123",
		},
	}
	trips := &discoveryTripRepository{
		tripID: "trip-123",
		trip: &models.Trip{
			BaseModel:     models.BaseModel{ID: "trip-123"},
			RideRequestID: "request-123",
			CustomerID:    "different-customer",
		},
	}
	service := newDiscoveryService(authorizer, trips)

	result, err := service.GetByRideRequestIDAuthorized(
		context.Background(), "request-123", "customer-123",
	)
	if !errors.Is(err, ErrTripAccessDenied) {
		t.Fatalf("expected trip access denial, got %v", err)
	}
	if result != nil {
		t.Fatalf("unauthorized trip disclosed: %#v", result)
	}
}

func TestTripDiscoveryRejectsRelationshipMismatch(t *testing.T) {
	authorizer := &discoveryAuthorizer{
		request: &models.RideRequest{
			BaseModel:  models.BaseModel{ID: "request-123"},
			CustomerID: "customer-123",
		},
	}
	trips := &discoveryTripRepository{
		tripID: "trip-123",
		trip: &models.Trip{
			BaseModel:     models.BaseModel{ID: "trip-123"},
			RideRequestID: "different-request",
			CustomerID:    "customer-123",
		},
	}
	service := newDiscoveryService(authorizer, trips)

	result, err := service.GetByRideRequestIDAuthorized(
		context.Background(), "request-123", "customer-123",
	)
	if err == nil {
		t.Fatal("expected relationship mismatch error")
	}
	if result != nil {
		t.Fatalf("mismatched trip disclosed: %#v", result)
	}
}
