package trip

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type readBoundaryTripRepository struct {
	repository.TripRepository
	trip  *models.Trip
	calls int
}

func (r *readBoundaryTripRepository) GetByID(
	_ context.Context,
	_ string,
) (*models.Trip, error) {
	r.calls++
	return r.trip, nil
}

type readBoundaryRolesRepository struct {
	repository.UserRoleRepository
	roles []string
}

func (r *readBoundaryRolesRepository) GetUserRoles(
	_ context.Context,
	_ string,
) ([]string, error) {
	return r.roles, nil
}

type readBoundaryEventRepository struct {
	repository.TripEventRepository
	calls int
}

func (r *readBoundaryEventRepository) ListByTripID(
	_ context.Context,
	_ string,
) ([]*models.TripEvent, error) {
	r.calls++
	return []*models.TripEvent{}, nil
}

func TestTripReadServiceBoundaryAuthorization(t *testing.T) {
	cases := []struct {
		name        string
		roles       []string
		userID      string
		driverID    string
		customerID  string
		member      bool
		wantAllowed bool
	}{
		{
			name:        "authorized company administrator",
			roles:       []string{"COMPANY_ADMIN"},
			userID:      "admin",
			member:      true,
			wantAllowed: true,
		},
		{
			name:        "authorized dispatcher",
			roles:       []string{"DISPATCHER"},
			userID:      "dispatcher",
			member:      true,
			wantAllowed: true,
		},
		{
			name:   "cross company administrator denied",
			roles:  []string{"COMPANY_ADMIN"},
			userID: "admin",
			member: false,
		},
		{
			name:   "revoked dispatcher denied",
			roles:  []string{"DISPATCHER"},
			userID: "dispatcher",
			member: false,
		},
		{
			name:        "system administrator allowed",
			roles:       []string{"SYSTEM_ADMIN"},
			userID:      "system-admin",
			wantAllowed: true,
		},
		{
			name:        "owning customer allowed",
			roles:       []string{"CUSTOMER"},
			userID:      "customer",
			customerID:  "customer",
			wantAllowed: true,
		},
		{
			name:        "assigned driver allowed",
			roles:       []string{"DRIVER"},
			userID:      "driver",
			driverID:    "driver",
			wantAllowed: true,
		},
		{
			name:       "unrelated customer denied",
			roles:      []string{"CUSTOMER"},
			userID:     "outsider",
			customerID: "customer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tripRepo := &readBoundaryTripRepository{
				trip: &models.Trip{
					BaseModel: models.BaseModel{
						ID: "trip-123",
					},
					CompanyID:  "company-a",
					DriverID:   tc.driverID,
					CustomerID: tc.customerID,
				},
			}

			memberships := &readTenantMembershipStub{
				allowed: tc.member,
			}

			events := &readBoundaryEventRepository{}

			service := NewService(Dependencies{
				Trips: tripRepo,
				UserRoles: &readBoundaryRolesRepository{
					roles: tc.roles,
				},
				CompanyMemberships: memberships,
				TripEvents:         events,
			})

			result, err := service.GetByIDAuthorized(
				context.Background(),
				"trip-123",
				tc.userID,
			)

			if tc.wantAllowed {
				if err != nil || result == nil {
					t.Fatalf(
						"expected authorized trip, got result=%v err=%v",
						result, err,
					)
				}
			} else {
				if !errors.Is(err, ErrTripAccessDenied) || result != nil {
					t.Fatalf(
						"expected trip access denied, got result=%v err=%v",
						result, err,
					)
				}
			}

			tripEvents, eventErr := service.ListEvents(
				context.Background(),
				"trip-123",
				tc.userID,
			)

			if tc.wantAllowed {
				if eventErr != nil {
					t.Fatalf("expected authorized events: %v", eventErr)
				}
				if tripEvents == nil || events.calls != 1 {
					t.Fatalf(
						"expected event repository call, events=%v calls=%d",
						tripEvents, events.calls,
					)
				}
			} else {
				if !errors.Is(eventErr, ErrTripEventAccessDenied) {
					t.Fatalf(
						"expected event access denied, got %v",
						eventErr,
					)
				}

				if events.calls != 0 || tripEvents != nil {
					t.Fatalf(
						"unauthorized events accessed: events=%v calls=%d",
						tripEvents, events.calls,
					)
				}
			}

			if tripRepo.calls != 2 {
				t.Fatalf(
					"expected two trip lookups, got %d",
					tripRepo.calls,
				)
			}
		})
	}
}

func TestTripReadServiceBoundaryMembershipFailure(t *testing.T) {
	membershipErr := errors.New("membership backend unavailable")

	memberships := &readTenantMembershipStub{
		err: membershipErr,
	}

	events := &readBoundaryEventRepository{}

	service := NewService(Dependencies{
		Trips: &readBoundaryTripRepository{
			trip: &models.Trip{
				CompanyID: "company-a",
			},
		},
		UserRoles: &readBoundaryRolesRepository{
			roles: []string{"DISPATCHER"},
		},
		CompanyMemberships: memberships,
		TripEvents:         events,
	})

	tripResult, err := service.GetByIDAuthorized(
		context.Background(),
		"trip-123",
		"dispatcher",
	)

	if tripResult != nil || !errors.Is(err, membershipErr) {
		t.Fatalf(
			"expected trip read to fail closed: result=%v err=%v",
			tripResult, err,
		)
	}

	tripEvents, err := service.ListEvents(
		context.Background(),
		"trip-123",
		"dispatcher",
	)

	if tripEvents != nil || !errors.Is(err, membershipErr) {
		t.Fatalf(
			"expected event read to fail closed: events=%v err=%v",
			tripEvents, err,
		)
	}

	if events.calls != 0 {
		t.Fatalf(
			"event repository accessed after membership failure: %d",
			events.calls,
		)
	}
}
