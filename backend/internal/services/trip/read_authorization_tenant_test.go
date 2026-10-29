package trip

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type readTenantMembershipStub struct {
	repository.CompanyMembershipRepository
	allowed bool
	err     error
	calls   int
}

func (r *readTenantMembershipStub) Exists(
	ctx context.Context,
	userID string,
	companyID string,
) (bool, error) {
	r.calls++
	return r.allowed, r.err
}

func TestTripReadTenantAuthorization(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name                string
		roles               []string
		userID              string
		companyID           string
		driverID            string
		customerID          string
		member              bool
		wantAllowed         bool
		wantMembershipCalls int
	}{
		{
			name:                "company administrator with membership",
			roles:               []string{"COMPANY_ADMIN"},
			userID:              "admin",
			companyID:           "company-a",
			member:              true,
			wantAllowed:         true,
			wantMembershipCalls: 1,
		},
		{
			name:                "dispatcher with membership",
			roles:               []string{"DISPATCHER"},
			userID:              "dispatcher",
			companyID:           "company-a",
			member:              true,
			wantAllowed:         true,
			wantMembershipCalls: 1,
		},
		{
			name:                "cross company administrator denied",
			roles:               []string{"COMPANY_ADMIN"},
			userID:              "admin",
			companyID:           "company-b",
			member:              false,
			wantAllowed:         false,
			wantMembershipCalls: 1,
		},
		{
			name:                "revoked dispatcher denied",
			roles:               []string{"DISPATCHER"},
			userID:              "dispatcher",
			companyID:           "company-a",
			member:              false,
			wantAllowed:         false,
			wantMembershipCalls: 1,
		},
		{
			name:        "system administrator globally authorized",
			roles:       []string{"SYSTEM_ADMIN"},
			userID:      "system-admin",
			companyID:   "company-b",
			wantAllowed: true,
		},
		{
			name:        "owning customer authorized",
			roles:       []string{"CUSTOMER"},
			userID:      "customer",
			companyID:   "company-a",
			customerID:  "customer",
			wantAllowed: true,
		},
		{
			name:        "assigned driver authorized",
			roles:       []string{"DRIVER"},
			userID:      "driver",
			companyID:   "company-a",
			driverID:    "driver",
			wantAllowed: true,
		},
		{
			name:        "unrelated customer denied",
			roles:       []string{"CUSTOMER"},
			userID:      "outsider",
			companyID:   "company-a",
			customerID:  "customer",
			wantAllowed: false,
		},
		{
			name:        "unassigned driver denied",
			roles:       []string{"DRIVER"},
			userID:      "outsider",
			companyID:   "company-a",
			driverID:    "driver",
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			membership := &readTenantMembershipStub{
				allowed: tt.member,
			}

			service := &tripService{
				companyMemberships: membership,
			}

			trip := &models.Trip{
				CompanyID:  tt.companyID,
				DriverID:   tt.driverID,
				CustomerID: tt.customerID,
			}

			allowed, err := service.canViewTripAuthorized(
				ctx,
				tt.roles,
				tt.userID,
				trip,
			)

			if err != nil {
				t.Fatalf("authorization error: %v", err)
			}

			if allowed != tt.wantAllowed {
				t.Fatalf(
					"allowed = %t, want %t",
					allowed,
					tt.wantAllowed,
				)
			}

			if membership.calls != tt.wantMembershipCalls {
				t.Fatalf(
					"membership calls = %d, want %d",
					membership.calls,
					tt.wantMembershipCalls,
				)
			}
		})
	}
}

func TestTripReadTenantAuthorizationFailsClosed(t *testing.T) {
	ctx := context.Background()

	trip := &models.Trip{
		CompanyID: "company-a",
	}

	t.Run("missing membership repository", func(t *testing.T) {
		service := &tripService{}

		allowed, err := service.canViewTripAuthorized(
			ctx,
			[]string{"COMPANY_ADMIN"},
			"admin",
			trip,
		)

		if allowed || err == nil {
			t.Fatalf(
				"expected fail closed, got allowed=%t err=%v",
				allowed,
				err,
			)
		}
	})

	t.Run("membership lookup error", func(t *testing.T) {
		lookupErr := errors.New("membership lookup failed")

		service := &tripService{
			companyMemberships: &readTenantMembershipStub{
				err: lookupErr,
			},
		}

		allowed, err := service.canViewTripAuthorized(
			ctx,
			[]string{"DISPATCHER"},
			"dispatcher",
			trip,
		)

		if allowed || !errors.Is(err, lookupErr) {
			t.Fatalf(
				"expected membership error, got allowed=%t err=%v",
				allowed,
				err,
			)
		}
	})
}
