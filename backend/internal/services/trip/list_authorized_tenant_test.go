package trip

import (
	"context"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

type tenantListRolesStub struct {
	repository.UserRoleRepository
	roles []string
}

func (r *tenantListRolesStub) GetUserRoles(
	ctx context.Context,
	userID string,
) ([]string, error) {
	return r.roles, nil
}

func TestTenantListOperationalRolesFailClosedWithoutDatabase(
	t *testing.T,
) {
	tests := []struct {
		name string
		role string
	}{
		{
			name: "company administrator",
			role: "COMPANY_ADMIN",
		},
		{
			name: "dispatcher",
			role: "DISPATCHER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &tripService{
				userRoles: &tenantListRolesStub{
					roles: []string{tt.role},
				},
			}

			_, err := service.ListAuthorized(
				context.Background(),
				"tenant-user",
				"",
				"",
				"",
				"",
				"",
				20,
				0,
			)

			if err == nil {
				t.Fatal("expected missing database to fail closed")
			}

			if !strings.Contains(
				err.Error(),
				"trip database is not configured",
			) {
				t.Fatalf(
					"unexpected error: %v",
					err,
				)
			}
		})
	}
}

func TestTenantListRejectsUserWithoutAuthorizedRole(
	t *testing.T,
) {
	service := &tripService{
		userRoles: &tenantListRolesStub{
			roles: []string{"UNRELATED_ROLE"},
		},
	}

	_, err := service.ListAuthorized(
		context.Background(),
		"unprivileged-user",
		"",
		"",
		"",
		"",
		"",
		20,
		0,
	)

	if err != ErrTripAccessDenied {
		t.Fatalf(
			"expected ErrTripAccessDenied, got %v",
			err,
		)
	}
}

func TestTenantListRejectsMissingUserIdentity(
	t *testing.T,
) {
	service := &tripService{}

	_, err := service.ListAuthorized(
		context.Background(),
		"",
		"",
		"",
		"",
		"",
		"",
		20,
		0,
	)

	if err == nil {
		t.Fatal("expected missing user identity to be rejected")
	}
}

func TestTenantListRejectsNegativePagination(
	t *testing.T,
) {
	service := &tripService{}

	tests := []struct {
		name   string
		limit  int
		offset int
	}{
		{
			name:  "negative limit",
			limit: -1,
		},
		{
			name:   "negative offset",
			limit:  20,
			offset: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.ListAuthorized(
				context.Background(),
				"tenant-user",
				"",
				"",
				"",
				"",
				"",
				tt.limit,
				tt.offset,
			)

			if err == nil {
				t.Fatal("expected invalid pagination to be rejected")
			}
		})
	}
}
