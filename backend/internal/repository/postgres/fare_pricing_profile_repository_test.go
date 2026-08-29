package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestFarePricingProfileRepositoryResolveEffective(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	const companyID = "345c5e3e-b07a-4e16-837d-e5d32254d6f3"

	branchID :=
		"186f7570-6902-41a2-a1f9-d509a4d90fcb"

	categoryRepo :=
		NewServiceCategoryRepository(db)

	pricingRepo :=
		NewFarePricingProfileRepository(db)

	category := &models.ServiceCategory{
		CompanyID: companyID,
		Code:      "PRICING_TEST_" + uuid.NewString()[:8],
		Name:      "Pricing Resolver Test",
		IsActive:  true,
	}

	category.ID = uuid.NewString()

	if err := categoryRepo.Create(
		ctx,
		category,
	); err != nil {
		t.Fatalf(
			"create service category: %v",
			err,
		)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`
				DELETE FROM fare_pricing_profiles
				WHERE service_category_id = $1
			`,
			category.ID,
		)

		_, _ = db.Exec(
			context.Background(),
			`
				DELETE FROM service_categories
				WHERE id = $1
			`,
			category.ID,
		)
	}()

	now := time.Now().UTC()

	companyProfile := &models.FarePricingProfile{
		ID:                uuid.NewString(),
		CompanyID:         companyID,
		BranchID:          nil,
		ServiceCategoryID: category.ID,
		Version:           "company-" + uuid.NewString(),
		Currency:          "EUR",

		BaseFare:             5.00,
		DistanceRatePerKM:    1.50,
		TimeRatePerMinute:    0.50,
		WaitingRatePerMinute: 0.40,
		BookingFee:           1.00,
		SurgeMultiplier:      1.00,

		EffectiveFrom: now.Add(-2 * time.Hour),

		IsActive: true,
	}

	if err := pricingRepo.Create(
		ctx,
		companyProfile,
	); err != nil {
		t.Fatalf(
			"create company pricing profile: %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 1. Company-wide pricing must be used as fallback.
	// ---------------------------------------------------------

	resolved, err :=
		pricingRepo.ResolveEffective(
			ctx,
			companyID,
			&branchID,
			category.ID,
			now,
		)
	if err != nil {
		t.Fatalf(
			"resolve company fallback: %v",
			err,
		)
	}

	if resolved.ID != companyProfile.ID {
		t.Fatalf(
			"expected company fallback %s, got %s",
			companyProfile.ID,
			resolved.ID,
		)
	}

	// ---------------------------------------------------------
	// 2. Branch-specific pricing must override company pricing.
	// ---------------------------------------------------------

	branchProfile := &models.FarePricingProfile{
		ID:                uuid.NewString(),
		CompanyID:         companyID,
		BranchID:          stringPtr(branchID),
		ServiceCategoryID: category.ID,
		Version:           "branch-" + uuid.NewString(),
		Currency:          "EUR",

		BaseFare:             7.00,
		DistanceRatePerKM:    2.00,
		TimeRatePerMinute:    0.75,
		WaitingRatePerMinute: 0.60,
		BookingFee:           2.00,
		SurgeMultiplier:      1.00,

		EffectiveFrom: now.Add(-time.Hour),

		IsActive: true,
	}

	if err := pricingRepo.Create(
		ctx,
		branchProfile,
	); err != nil {
		t.Fatalf(
			"create branch pricing profile: %v",
			err,
		)
	}

	resolved, err =
		pricingRepo.ResolveEffective(
			ctx,
			companyID,
			&branchID,
			category.ID,
			now,
		)
	if err != nil {
		t.Fatalf(
			"resolve branch override: %v",
			err,
		)
	}

	if resolved.ID != branchProfile.ID {
		t.Fatalf(
			"expected branch override %s, got %s",
			branchProfile.ID,
			resolved.ID,
		)
	}

	// ---------------------------------------------------------
	// 3. Future profile must be ignored.
	// ---------------------------------------------------------

	futureEnd :=
		now.Add(4 * time.Hour)

	futureProfile := &models.FarePricingProfile{
		ID:                uuid.NewString(),
		CompanyID:         companyID,
		BranchID:          stringPtr(branchID),
		ServiceCategoryID: category.ID,
		Version:           "future-" + uuid.NewString(),
		Currency:          "EUR",

		BaseFare:             99.00,
		DistanceRatePerKM:    99.00,
		TimeRatePerMinute:    99.00,
		WaitingRatePerMinute: 99.00,
		BookingFee:           99.00,
		SurgeMultiplier:      1.00,

		EffectiveFrom: now.Add(3 * time.Hour),

		EffectiveTo: &futureEnd,

		IsActive: true,
	}

	if err := pricingRepo.Create(
		ctx,
		futureProfile,
	); err != nil {
		t.Fatalf(
			"create future pricing profile: %v",
			err,
		)
	}

	resolved, err =
		pricingRepo.ResolveEffective(
			ctx,
			companyID,
			&branchID,
			category.ID,
			now,
		)
	if err != nil {
		t.Fatalf(
			"resolve while future profile exists: %v",
			err,
		)
	}

	if resolved.ID != branchProfile.ID {
		t.Fatalf(
			"future profile incorrectly selected: got %s",
			resolved.ID,
		)
	}

	// ---------------------------------------------------------
	// 4. Expired profile must be ignored.
	// ---------------------------------------------------------

	expiredEnd :=
		now.Add(-3 * time.Hour)

	expiredProfile := &models.FarePricingProfile{
		ID:                uuid.NewString(),
		CompanyID:         companyID,
		BranchID:          stringPtr(branchID),
		ServiceCategoryID: category.ID,
		Version:           "expired-" + uuid.NewString(),
		Currency:          "EUR",

		BaseFare:             88.00,
		DistanceRatePerKM:    88.00,
		TimeRatePerMinute:    88.00,
		WaitingRatePerMinute: 88.00,
		BookingFee:           88.00,
		SurgeMultiplier:      1.00,

		EffectiveFrom: now.Add(-4 * time.Hour),

		EffectiveTo: &expiredEnd,

		IsActive: true,
	}

	if err := pricingRepo.Create(
		ctx,
		expiredProfile,
	); err != nil {
		t.Fatalf(
			"create expired pricing profile: %v",
			err,
		)
	}

	resolved, err =
		pricingRepo.ResolveEffective(
			ctx,
			companyID,
			&branchID,
			category.ID,
			now,
		)
	if err != nil {
		t.Fatalf(
			"resolve while expired profile exists: %v",
			err,
		)
	}

	if resolved.ID != branchProfile.ID {
		t.Fatalf(
			"expired profile incorrectly selected: got %s",
			resolved.ID,
		)
	}

	// ---------------------------------------------------------
	// 5. Verify GetByID.
	// ---------------------------------------------------------

	byID, err :=
		pricingRepo.GetByID(
			ctx,
			branchProfile.ID,
		)
	if err != nil {
		t.Fatalf(
			"get pricing profile by ID: %v",
			err,
		)
	}

	if byID.ID != branchProfile.ID {
		t.Fatalf(
			"expected profile ID %s, got %s",
			branchProfile.ID,
			byID.ID,
		)
	}

	// ---------------------------------------------------------
	// 6. Verify GetByVersion.
	// ---------------------------------------------------------

	byVersion, err :=
		pricingRepo.GetByVersion(
			ctx,
			companyID,
			branchProfile.Version,
		)
	if err != nil {
		t.Fatalf(
			"get pricing profile by version: %v",
			err,
		)
	}

	if byVersion.ID != branchProfile.ID {
		t.Fatalf(
			"expected profile ID %s by version, got %s",
			branchProfile.ID,
			byVersion.ID,
		)
	}

	// ---------------------------------------------------------
	// 7. ListByCompanyID must include our profiles.
	// ---------------------------------------------------------

	profiles, err :=
		pricingRepo.ListByCompanyID(
			ctx,
			companyID,
			true,
		)
	if err != nil {
		t.Fatalf(
			"list pricing profiles: %v",
			err,
		)
	}

	foundBranch := false

	for _, profile := range profiles {
		if profile.ID == branchProfile.ID {
			foundBranch = true
			break
		}
	}

	if !foundBranch {
		t.Fatal(
			"expected branch pricing profile in company list",
		)
	}
}

func stringPtr(
	value string,
) *string {
	return &value
}
