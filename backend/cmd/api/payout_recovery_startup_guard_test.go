package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDriverPayoutRecoveryNotWiredIntoAPIStartup(t *testing.T) {
	forbidden := map[string]bool{
		"NewPostgresPayoutRecoveryWorker": true,
		"PostgresPayoutRecoveryWorker":    true,
	}

	directories := []string{
		".",
		filepath.Join("..", "..", "internal", "api"),
	}

	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("read directory %s: %v", directory, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()

			if !strings.HasSuffix(name, ".go") ||
				strings.HasSuffix(name, "_test.go") {
				continue
			}

			path := filepath.Join(directory, name)

			file, err := parser.ParseFile(
				token.NewFileSet(),
				path,
				nil,
				0,
			)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}

			ast.Inspect(file, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok {
					return true
				}

				if forbidden[identifier.Name] {
					t.Errorf(
						"payout recovery must not be wired into API startup: %s references %s",
						path,
						identifier.Name,
					)
				}

				return true
			})
		}
	}
}
