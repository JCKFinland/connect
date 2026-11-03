package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestStripeProvisioningProductionStartupOrder verifies the production
// startup sequence and the feature-gated route composition.
// It performs no database writes or Stripe network operations.
func TestStripeProvisioningProductionStartupOrder(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read production startup: %v", err)
	}

	source := string(data)

	markers := []string{
		"validateStripeProvisioningActivation(provisioningEnabled)",
		"database.Connect(cfg)",
		"database.RunMigrations(cfg, log)",
		"database.VerifyStripeProvisioningSchema(",
		"router := api.NewRouter(",
		"assembleStripeProvisioningComposition(",
		"server := &http.Server{",
		"server.ListenAndServe()",
	}

	previous := -1
	for _, marker := range markers {
		if count := strings.Count(source, marker); count != 1 {
			t.Fatalf("expected one startup marker %q, got %d", marker, count)
		}

		position := strings.Index(source, marker)
		if position <= previous {
			t.Fatalf("startup ordering violated at %q", marker)
		}
		previous = position
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", data, 0)
	if err != nil {
		t.Fatalf("parse production startup: %v", err)
	}

	var run *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "run" {
			if run != nil {
				t.Fatal("multiple run functions")
			}
			run = fn
		}
	}

	if run == nil || run.Body == nil {
		t.Fatal("production run function missing")
	}

	compositionCount := 0
	guardedCount := 0

	ast.Inspect(run.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == "assembleStripeProvisioningComposition" {
			compositionCount++
		}
		return true
	})

	ast.Inspect(run.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}

		condition, ok := statement.Cond.(*ast.Ident)
		if !ok || condition.Name != "provisioningEnabled" {
			return true
		}

		ast.Inspect(statement.Body, func(child ast.Node) bool {
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}

			ident, ok := call.Fun.(*ast.Ident)
			if ok && ident.Name == "assembleStripeProvisioningComposition" {
				guardedCount++
			}
			return true
		})

		return true
	})

	if compositionCount != 1 || guardedCount != 1 {
		t.Fatalf(
			"expected one feature-gated composition call, total=%d guarded=%d",
			compositionCount,
			guardedCount,
		)
	}

	if !strings.Contains(
		source,
		"Stripe provisioning route assembly failed",
	) {
		t.Fatal("composition failure handling missing")
	}
}
