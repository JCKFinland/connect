package trip

import (
	"context"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tenantListMembershipStub struct {
	repository.CompanyMembershipRepository
}
type tenantListUnsupportedRepository struct {
	repository.TripRepository
}

func TestTenantListRejectsRepositoryWithoutTenantCapability(t *testing.T) {
	for _, role := range []string{
		"COMPANY_ADMIN",
		"DISPATCHER",
	} {
		t.Run(role, func(t *testing.T) {
			service := &tripService{
				db:   &pgxpool.Pool{},
				repo: &tenantListUnsupportedRepository{},
				userRoles: &tenantListRolesStub{
					roles: []string{role},
				},
				companyMemberships: &tenantListMembershipStub{},
			}

			_, err := service.ListAuthorized(
				context.Background(),
				"00000000-0000-4000-8000-000000000001",
				"", "", "", "", "", 10, 0,
			)

			if err == nil {
				t.Fatal("expected authorization failure")
			}

			if !strings.Contains(
				err.Error(),
				"trip repository does not support tenant-authorized listing",
			) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
