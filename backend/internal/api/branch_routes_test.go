package api

import (
	"os"
	"strings"
	"testing"
)

func TestBranchRoutesEnforceRBACPermissions(t *testing.T) {
	sourceBytes, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}

	source := string(sourceBytes)

	start := strings.Index(
		source,
		`branches := v1.Group("/branches")`,
	)
	if start == -1 {
		t.Fatal("branch route group not found")
	}

	end := strings.Index(
		source[start:],
		`fleets := v1.Group("/fleets")`,
	)
	if end == -1 {
		t.Fatal("fleet route group not found after branch route group")
	}

	branchRoutes := source[start : start+end]

	required := []string{
		`branches.Use(authMiddleware.RequireAuth())`,

		`branches.POST(
				"",
				rbacMiddleware.RequirePermission("branches.manage"),
				branchHandler.Create,
			)`,

		`branches.GET(
				"",
				rbacMiddleware.RequirePermission("branches.read"),
				branchHandler.List,
			)`,

		`branches.GET(
				"/:id",
				rbacMiddleware.RequirePermission("branches.read"),
				branchHandler.GetByID,
			)`,

		`branches.PUT(
				"/:id",
				rbacMiddleware.RequirePermission("branches.manage"),
				branchHandler.Update,
			)`,

		`branches.DELETE(
				"/:id",
				rbacMiddleware.RequirePermission("branches.manage"),
				branchHandler.Delete,
			)`,

		`branches.PATCH(
				"/:id/deactivate",
				rbacMiddleware.RequirePermission("branches.manage"),
				branchHandler.Deactivate,
			)`,

		`branches.PATCH(
				"/:id/reactivate",
				rbacMiddleware.RequirePermission("branches.manage"),
				branchHandler.Reactivate,
			)`,
	}

	for _, expected := range required {
		if !strings.Contains(branchRoutes, expected) {
			t.Errorf(
				"branch route group is missing required RBAC wiring:\n%s",
				expected,
			)
		}
	}

	if strings.Count(
		branchRoutes,
		`RequirePermission("branches.read")`,
	) != 2 {
		t.Errorf(
			"expected exactly 2 branches.read route guards, got %d",
			strings.Count(
				branchRoutes,
				`RequirePermission("branches.read")`,
			),
		)
	}

	if strings.Count(
		branchRoutes,
		`RequirePermission("branches.manage")`,
	) != 5 {
		t.Errorf(
			"expected exactly 5 branches.manage route guards, got %d",
			strings.Count(
				branchRoutes,
				`RequirePermission("branches.manage")`,
			),
		)
	}
}
