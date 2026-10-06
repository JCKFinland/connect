package api

import (
	"os"
	"strings"
	"testing"
)

func TestCompanyRoutesEnforceRBACPermissions(t *testing.T) {
	sourceBytes, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}

	source := string(sourceBytes)

	start := strings.Index(
		source,
		`companies := v1.Group("/companies")`,
	)
	if start == -1 {
		t.Fatal("company route group not found")
	}

	end := strings.Index(
		source[start:],
		`branches := v1.Group("/branches")`,
	)
	if end == -1 {
		t.Fatal("branch route group not found after company route group")
	}

	companyRoutes := source[start : start+end]

	required := []string{
		`companies.Use(authMiddleware.RequireAuth())`,

		`companies.POST(
				"",
				rbacMiddleware.RequirePermission("companies.manage"),
				companyHandler.Create,
			)`,

		`companies.GET(
				"",
				rbacMiddleware.RequirePermission("companies.read"),
				companyHandler.List,
			)`,

		`companies.GET(
				"/:id",
				rbacMiddleware.RequirePermission("companies.read"),
				companyHandler.GetByID,
			)`,

		`companies.PUT(
				"/:id",
				rbacMiddleware.RequirePermission("companies.manage"),
				companyHandler.Update,
			)`,

		`companies.DELETE(
				"/:id",
				rbacMiddleware.RequirePermission("companies.manage"),
				companyHandler.Delete,
			)`,

		`companies.PATCH(
				"/:id/deactivate",
				rbacMiddleware.RequirePermission("companies.manage"),
				companyHandler.Deactivate,
			)`,

		`companies.PATCH(
				"/:id/reactivate",
				rbacMiddleware.RequirePermission("companies.manage"),
				companyHandler.Reactivate,
			)`,
	}

	for _, expected := range required {
		if !strings.Contains(companyRoutes, expected) {
			t.Errorf(
				"company route group is missing required RBAC wiring:\n%s",
				expected,
			)
		}
	}

	if strings.Count(
		companyRoutes,
		`RequirePermission("companies.read")`,
	) != 2 {
		t.Errorf(
			"expected exactly 2 companies.read route guards, got %d",
			strings.Count(
				companyRoutes,
				`RequirePermission("companies.read")`,
			),
		)
	}

	if strings.Count(
		companyRoutes,
		`RequirePermission("companies.manage")`,
	) != 5 {
		t.Errorf(
			"expected exactly 5 companies.manage route guards, got %d",
			strings.Count(
				companyRoutes,
				`RequirePermission("companies.manage")`,
			),
		)
	}
}
