package trip

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type gpsTenantMembershipStub struct {
	repository.CompanyMembershipRepository

	allowed   bool
	err       error
	calls     int
	userID    string
	companyID string
}

func (m *gpsTenantMembershipStub) Exists(
	_ context.Context,
	userID string,
	companyID string,
) (bool, error) {
	m.calls++
	m.userID = userID
	m.companyID = companyID

	if m.err != nil {
		return false, m.err
	}

	return m.allowed, nil
}

func TestListTripLocationsTenantAuthorization(t *testing.T) {
	const (
		tripID    = "gps-trip-123"
		companyID = "company-123"
		staffID   = "staff-123"
	)

	cases := []struct {
		name                string
		role                string
		membership          bool
		membershipErr       error
		configureMembership bool
		wantAllowed         bool
		wantAccessDenied    bool
		wantMembershipCalls int
	}{
		{
			name:                "active company administrator",
			role:                "COMPANY_ADMIN",
			membership:          true,
			configureMembership: true,
			wantAllowed:         true,
			wantMembershipCalls: 1,
		},
		{
			name:                "active dispatcher",
			role:                "DISPATCHER",
			membership:          true,
			configureMembership: true,
			wantAllowed:         true,
			wantMembershipCalls: 1,
		},
		{
			name:                "revoked company administrator",
			role:                "COMPANY_ADMIN",
			configureMembership: true,
			wantAccessDenied:    true,
			wantMembershipCalls: 1,
		},
		{
			name:                "revoked dispatcher",
			role:                "DISPATCHER",
			configureMembership: true,
			wantAccessDenied:    true,
			wantMembershipCalls: 1,
		},
		{
			name:                "cross company administrator",
			role:                "COMPANY_ADMIN",
			configureMembership: true,
			wantAccessDenied:    true,
			wantMembershipCalls: 1,
		},
		{
			name:                "membership lookup failure",
			role:                "DISPATCHER",
			membershipErr:       errors.New("membership database unavailable"),
			configureMembership: true,
			wantMembershipCalls: 1,
		},
		{
			name: "missing membership repository",
			role: "COMPANY_ADMIN",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tripRepo := &listLocationsTripRepository{
				trip: &models.Trip{
					BaseModel: models.BaseModel{
						ID: tripID,
					},
					CompanyID:  companyID,
					CustomerID: "customer-owner",
					DriverID:   "driver-owner",
				},
			}

			roleRepo := &listLocationsUserRoleRepository{
				roles: []string{tc.role},
			}

			locationRepo := &listLocationsRepository{
				locations: []*models.TripLocation{
					{
						TripID:    tripID,
						DriverID:  "driver-owner",
						Latitude:  60.1699,
						Longitude: 24.9384,
					},
				},
			}

			memberships := &gpsTenantMembershipStub{
				allowed: tc.membership,
				err:     tc.membershipErr,
			}

			dependencies := Dependencies{
				Trips:         tripRepo,
				UserRoles:     roleRepo,
				TripLocations: locationRepo,
			}

			if tc.configureMembership {
				dependencies.CompanyMemberships = memberships
			}

			service := NewService(dependencies)

			locations, err := service.ListTripLocations(
				context.Background(),
				tripID,
				staffID,
			)

			if tc.wantAllowed {
				if err != nil {
					t.Fatalf("expected authorized GPS access: %v", err)
				}

				if len(locations) != 1 {
					t.Fatalf("expected one GPS sample, got %d", len(locations))
				}

				if !locationRepo.called {
					t.Fatal("authorized request must query GPS repository")
				}
			} else {
				if err == nil {
					t.Fatal("expected GPS access to fail")
				}

				if tc.wantAccessDenied &&
					!errors.Is(err, ErrTripAccessDenied) {
					t.Fatalf("expected ErrTripAccessDenied, got %v", err)
				}

				if locations != nil {
					t.Fatalf("denied request returned GPS data: %+v", locations)
				}

				if locationRepo.called {
					t.Fatal("GPS repository queried after authorization failure")
				}
			}

			if memberships.calls != tc.wantMembershipCalls {
				t.Fatalf(
					"expected %d membership checks, got %d",
					tc.wantMembershipCalls,
					memberships.calls,
				)
			}

			if tc.wantMembershipCalls > 0 {
				if memberships.userID != staffID ||
					memberships.companyID != companyID {
					t.Fatalf(
						"membership checked wrong identity: user=%q company=%q",
						memberships.userID,
						memberships.companyID,
					)
				}
			}
		})
	}
}
