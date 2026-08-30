package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestServiceCategoryRepositoryRoundTrip(
	t *testing.T,
) {
	ctx := context.Background()

	// ---------------------------------------------------------
	// 1. Run from backend root so config.Load() finds .env.
	// ---------------------------------------------------------

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	// ---------------------------------------------------------
	// 2. Load CONNECT configuration and database.
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	repo := NewServiceCategoryRepository(db)

	// ---------------------------------------------------------
	// 3. Create disposable platform-level category.
	// ---------------------------------------------------------

	categoryID := uuid.NewString()

	description :=
		"Disposable pricing repository integration category"

	category := &models.ServiceCategory{
		Code:        "TEST_" + uuid.NewString()[:8],
		Name:        "Repository Test Category",
		Description: &description,
		IsActive:    true,
	}

	category.ID = categoryID

	err = repo.Create(
		ctx,
		category,
	)
	if err != nil {
		t.Fatalf(
			"create service category: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM service_categories
				WHERE id = $1
			`,
			categoryID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup service category: %v",
				cleanupErr,
			)
		}
	}()

	if category.CreatedAt.IsZero() {
		t.Fatal(
			"expected created_at to be populated",
		)
	}

	if category.UpdatedAt.IsZero() {
		t.Fatal(
			"expected updated_at to be populated",
		)
	}

	// ---------------------------------------------------------
	// 4. GetByID round trip.
	// ---------------------------------------------------------

	byID, err := repo.GetByID(
		ctx,
		categoryID,
	)
	if err != nil {
		t.Fatalf(
			"get service category by ID: %v",
			err,
		)
	}

	if byID.ID != categoryID {
		t.Fatalf(
			"expected category ID %s, got %s",
			categoryID,
			byID.ID,
		)
	}

	if byID.Code != category.Code {
		t.Fatalf(
			"expected code %s, got %s",
			category.Code,
			byID.Code,
		)
	}

	if byID.Name != category.Name {
		t.Fatalf(
			"expected name %s, got %s",
			category.Name,
			byID.Name,
		)
	}

	if byID.Description == nil ||
		*byID.Description != description {
		t.Fatalf(
			"unexpected description: %v",
			byID.Description,
		)
	}

	if !byID.IsActive {
		t.Fatal(
			"expected category to be active",
		)
	}

	// ---------------------------------------------------------
	// 5. GetByCode must be platform-wide and case-insensitive.
	// ---------------------------------------------------------

	byCode, err := repo.GetByCode(
		ctx,
		stringLower(category.Code),
	)
	if err != nil {
		t.Fatalf(
			"get service category by code: %v",
			err,
		)
	}

	if byCode.ID != categoryID {
		t.Fatalf(
			"expected category ID %s from case-insensitive lookup, got %s",
			categoryID,
			byCode.ID,
		)
	}

	// ---------------------------------------------------------
	// 6. List(activeOnly=true).
	// ---------------------------------------------------------

	activeCategories, err := repo.List(
		ctx,
		true,
	)
	if err != nil {
		t.Fatalf(
			"list active service categories: %v",
			err,
		)
	}

	foundActive := false

	for _, item := range activeCategories {
		if item.ID == categoryID {
			foundActive = true
			break
		}
	}

	if !foundActive {
		t.Fatal(
			"expected active category in active-only list",
		)
	}

	// ---------------------------------------------------------
	// 7. Mark category inactive directly for repository filtering.
	// ---------------------------------------------------------

	_, err = db.Exec(
		ctx,
		`
			UPDATE service_categories
			SET
				is_active = FALSE,
				updated_at = NOW()
			WHERE id = $1
		`,
		categoryID,
	)
	if err != nil {
		t.Fatalf(
			"mark category inactive: %v",
			err,
		)
	}

	activeCategories, err = repo.List(
		ctx,
		true,
	)
	if err != nil {
		t.Fatalf(
			"list active categories after deactivation: %v",
			err,
		)
	}

	for _, item := range activeCategories {
		if item.ID == categoryID {
			t.Fatal(
				"inactive category unexpectedly returned by active-only list",
			)
		}
	}

	// ---------------------------------------------------------
	// 8. activeOnly=false must still return historical/inactive
	// category.
	// ---------------------------------------------------------

	allCategories, err := repo.List(
		ctx,
		false,
	)
	if err != nil {
		t.Fatalf(
			"list all service categories: %v",
			err,
		)
	}

	foundInactive := false

	for _, item := range allCategories {
		if item.ID == categoryID {
			foundInactive = true

			if item.IsActive {
				t.Fatal(
					"expected returned category to be inactive",
				)
			}

			break
		}
	}

	if !foundInactive {
		t.Fatal(
			"expected inactive category in full list",
		)
	}

	// ---------------------------------------------------------
	// 9. Duplicate platform category code must fail
	// case-insensitively.
	// ---------------------------------------------------------

	duplicate := &models.ServiceCategory{
		Code:     stringLower(category.Code),
		Name:     "Duplicate Category",
		IsActive: true,
	}

	err = repo.Create(
		ctx,
		duplicate,
	)
	if err == nil {
		t.Fatal(
			"expected duplicate platform category code to fail",
		)
	}
}

func stringLower(
	value string,
) string {
	result := make(
		[]byte,
		len(value),
	)

	for i := range value {
		ch := value[i]

		if ch >= 'A' && ch <= 'Z' {
			result[i] =
				ch + ('a' - 'A')
		} else {
			result[i] = ch
		}
	}

	return string(result)
}
