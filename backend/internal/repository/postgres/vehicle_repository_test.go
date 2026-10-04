package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestVehicleRepositoryEnforcesCompanyMembershipReadAuthority(
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
	t.Cleanup(func() {
		db.Close()
	})

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})

	memberUserID := uuid.NewString()
	noMembershipUserID := uuid.NewString()
	companyOneID := uuid.NewString()
	companyTwoID := uuid.NewString()
	branchOneID := uuid.NewString()
	branchTwoID := uuid.NewString()
	fleetOneID := uuid.NewString()
	fleetTwoID := uuid.NewString()
	vehicleOneID := uuid.NewString()
	vehicleTwoID := uuid.NewString()

	createUser := func(
		userID string,
		label string,
	) {
		t.Helper()

		_, err := tx.Exec(
			ctx,
			`
				INSERT INTO users (
					id,
					email,
					password_hash,
					first_name,
					last_name
				)
				VALUES ($1, $2, $3, $4, $5)
			`,
			userID,
			label+"-"+userID+"@example.test",
			"test-password-hash",
			"Vehicle",
			"ReadAuthority",
		)
		if err != nil {
			t.Fatalf("create user %s: %v", label, err)
		}
	}

	createCompanyHierarchy := func(
		companyID string,
		branchID string,
		fleetID string,
		vehicleID string,
		label string,
	) {
		t.Helper()

		_, err := tx.Exec(
			ctx,
			`
				INSERT INTO companies (
					id,
					name,
					legal_name,
					business_id,
					email,
					country_code,
					timezone
				)
				VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
			`,
			companyID,
			"Vehicle Read "+label,
			"Vehicle Read "+label+" Oy",
			"VR-"+companyID[:8],
			companyID+"@example.test",
		)
		if err != nil {
			t.Fatalf("create company %s: %v", label, err)
		}

		_, err = tx.Exec(
			ctx,
			`
				INSERT INTO branches (
					id,
					company_id,
					code,
					name
				)
				VALUES ($1, $2, $3, $4)
			`,
			branchID,
			companyID,
			"BR-"+branchID[:8],
			"Vehicle Read Branch "+label,
		)
		if err != nil {
			t.Fatalf("create branch %s: %v", label, err)
		}

		_, err = tx.Exec(
			ctx,
			`
				INSERT INTO fleets (
					id,
					company_id,
					branch_id,
					code,
					name
				)
				VALUES ($1, $2, $3, $4, $5)
			`,
			fleetID,
			companyID,
			branchID,
			"FL-"+fleetID[:8],
			"Vehicle Read Fleet "+label,
		)
		if err != nil {
			t.Fatalf("create fleet %s: %v", label, err)
		}

		_, err = tx.Exec(
			ctx,
			`
				INSERT INTO vehicles (
					id,
					company_id,
					branch_id,
					fleet_id,
					registration_number,
					make,
					model,
					model_year,
					color,
					vehicle_type,
					fuel_type,
					seating_capacity,
					is_active
				)
				VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10,
					$11, $12, $13
				)
			`,
			vehicleID,
			companyID,
			branchID,
			fleetID,
			"VR-"+vehicleID[:8],
			"Toyota",
			"Corolla",
			2025,
			"Black",
			"SEDAN",
			"HYBRID",
			4,
			true,
		)
		if err != nil {
			t.Fatalf("create vehicle %s: %v", label, err)
		}
	}

	createUser(memberUserID, "member")
	createUser(noMembershipUserID, "no-membership")

	createCompanyHierarchy(
		companyOneID,
		branchOneID,
		fleetOneID,
		vehicleOneID,
		"One",
	)
	createCompanyHierarchy(
		companyTwoID,
		branchTwoID,
		fleetTwoID,
		vehicleTwoID,
		"Two",
	)

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO company_memberships (
				user_id,
				company_id
			)
			VALUES ($1, $2)
		`,
		memberUserID,
		companyOneID,
	)
	if err != nil {
		t.Fatalf("create company membership: %v", err)
	}

	repo := &VehicleRepository{
		db: tx,
	}

	ownVehicle, err := repo.GetByIDForCompanyMember(
		ctx,
		memberUserID,
		vehicleOneID,
	)
	if err != nil {
		t.Fatalf("get own-company vehicle: %v", err)
	}
	if ownVehicle.ID != vehicleOneID {
		t.Fatalf(
			"expected own-company vehicle %s, got %s",
			vehicleOneID,
			ownVehicle.ID,
		)
	}

	_, err = repo.GetByIDForCompanyMember(
		ctx,
		memberUserID,
		vehicleTwoID,
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound for cross-company vehicle, got %v",
			err,
		)
	}

	memberVehicles, err := repo.ListForCompanyMember(
		ctx,
		memberUserID,
	)
	if err != nil {
		t.Fatalf("list member vehicles: %v", err)
	}
	if len(memberVehicles) != 1 {
		t.Fatalf(
			"expected exactly 1 membership-scoped vehicle, got %d",
			len(memberVehicles),
		)
	}
	if memberVehicles[0].ID != vehicleOneID {
		t.Fatalf(
			"expected only vehicle %s, got %s",
			vehicleOneID,
			memberVehicles[0].ID,
		)
	}
	if memberVehicles[0].CompanyID != companyOneID {
		t.Fatalf(
			"expected only company %s, got %s",
			companyOneID,
			memberVehicles[0].CompanyID,
		)
	}

	noMembershipVehicles, err := repo.ListForCompanyMember(
		ctx,
		noMembershipUserID,
	)
	if err != nil {
		t.Fatalf("list vehicles without membership: %v", err)
	}
	if len(noMembershipVehicles) != 0 {
		t.Fatalf(
			"user without company membership must see 0 vehicles, got %d",
			len(noMembershipVehicles),
		)
	}

	globalVehicles, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list global vehicles: %v", err)
	}

	foundOne := false
	foundTwo := false

	for _, vehicle := range globalVehicles {
		switch vehicle.ID {
		case vehicleOneID:
			foundOne = true
		case vehicleTwoID:
			foundTwo = true
		}
	}

	if !foundOne || !foundTwo {
		t.Fatalf(
			"global repository read must retain both test vehicles: vehicleOne=%t vehicleTwo=%t",
			foundOne,
			foundTwo,
		)
	}

	// A membership-guarded descriptive update must change only descriptive
	// fields. Hostile authority values on the model must never reach SQL.
	hostileCompanyID := uuid.NewString()
	hostileBranchID := uuid.NewString()
	hostileFleetID := uuid.NewString()
	updatedVIN := "UPDATEDVIN1234567"

	err = repo.UpdateDetailsForCompanyMember(
		ctx,
		memberUserID,
		&models.Vehicle{
			BaseModel: models.BaseModel{
				ID: vehicleOneID,
			},
			CompanyID:          hostileCompanyID,
			BranchID:           hostileBranchID,
			FleetID:            hostileFleetID,
			RegistrationNumber: "UPD-123",
			VIN:                &updatedVIN,
			Make:               "Volvo",
			Model:              "EX30",
			ModelYear:          2026,
			Color:              "Blue",
			VehicleType:        "SUV",
			FuelType:           "EV",
			SeatingCapacity:    5,
			IsActive:           false,
		},
	)
	if err != nil {
		t.Fatalf("membership-guarded descriptive update: %v", err)
	}

	updatedVehicle, err := repo.GetByID(ctx, vehicleOneID)
	if err != nil {
		t.Fatalf("get vehicle after descriptive update: %v", err)
	}

	if updatedVehicle.RegistrationNumber != "UPD-123" ||
		updatedVehicle.VIN == nil ||
		*updatedVehicle.VIN != updatedVIN ||
		updatedVehicle.Make != "Volvo" ||
		updatedVehicle.Model != "EX30" ||
		updatedVehicle.ModelYear != 2026 ||
		updatedVehicle.Color != "Blue" ||
		updatedVehicle.VehicleType != "SUV" ||
		updatedVehicle.FuelType != "EV" ||
		updatedVehicle.SeatingCapacity != 5 {
		t.Fatalf(
			"descriptive fields were not updated as expected: %#v",
			updatedVehicle,
		)
	}

	if updatedVehicle.CompanyID != companyOneID {
		t.Fatalf(
			"company authority changed: expected %s, got %s",
			companyOneID,
			updatedVehicle.CompanyID,
		)
	}
	if updatedVehicle.BranchID != branchOneID {
		t.Fatalf(
			"branch authority changed: expected %s, got %s",
			branchOneID,
			updatedVehicle.BranchID,
		)
	}
	if updatedVehicle.FleetID != fleetOneID {
		t.Fatalf(
			"fleet authority changed: expected %s, got %s",
			fleetOneID,
			updatedVehicle.FleetID,
		)
	}
	if !updatedVehicle.IsActive {
		t.Fatal("activation authority changed through descriptive update")
	}

	// Membership in company one must not authorize mutation of company two.
	err = repo.UpdateDetailsForCompanyMember(
		ctx,
		memberUserID,
		&models.Vehicle{
			BaseModel: models.BaseModel{
				ID: vehicleTwoID,
			},
			RegistrationNumber: "DENIED-1",
			Make:               "Denied",
			Model:              "Denied",
			ModelYear:          2026,
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound for cross-company update, got %v",
			err,
		)
	}

	vehicleTwoAfterDeniedUpdate, err := repo.GetByID(ctx, vehicleTwoID)
	if err != nil {
		t.Fatalf("get cross-company vehicle after denied update: %v", err)
	}
	if vehicleTwoAfterDeniedUpdate.RegistrationNumber != "VR-"+vehicleTwoID[:8] {
		t.Fatalf(
			"cross-company vehicle was mutated: registration=%s",
			vehicleTwoAfterDeniedUpdate.RegistrationNumber,
		)
	}

	// A user with no membership must not mutate even an otherwise valid target.
	err = repo.UpdateDetailsForCompanyMember(
		ctx,
		noMembershipUserID,
		&models.Vehicle{
			BaseModel: models.BaseModel{
				ID: vehicleOneID,
			},
			RegistrationNumber: "DENIED-2",
			Make:               "Denied",
			Model:              "Denied",
			ModelYear:          2026,
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound without company membership, got %v",
			err,
		)
	}

	vehicleOneAfterDeniedUpdate, err := repo.GetByID(ctx, vehicleOneID)
	if err != nil {
		t.Fatalf("get own-company vehicle after denied update: %v", err)
	}
	if vehicleOneAfterDeniedUpdate.RegistrationNumber != "UPD-123" {
		t.Fatalf(
			"vehicle changed after no-membership update: registration=%s",
			vehicleOneAfterDeniedUpdate.RegistrationNumber,
		)
	}
}
