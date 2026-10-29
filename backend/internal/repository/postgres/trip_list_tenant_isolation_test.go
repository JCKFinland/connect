package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTenantListCrossCompanyIsolation(t *testing.T) {
	const safeDatabase = "connect_integration_test"

	if os.Getenv("DB_NAME") != safeDatabase {
		t.Fatalf("refusing unsafe database: %q", os.Getenv("DB_NAME"))
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	backendRoot := filepath.Clean(filepath.Join(originalDir, "../../.."))

	if err := os.Chdir(backendRoot); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore directory: %v", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Name != safeDatabase {
		t.Fatalf("unsafe database: %q", cfg.Database.Name)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	companyA, cleanupA, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("company A fixture: %v", err)
	}
	defer func() {
		if err := cleanupA(context.Background()); err != nil {
			t.Errorf("cleanup company A: %v", err)
		}
	}()

	companyB, cleanupB, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("company B fixture: %v", err)
	}
	defer func() {
		if err := cleanupB(context.Background()); err != nil {
			t.Errorf("cleanup company B: %v", err)
		}
	}()

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup customer: %v", err)
		}
	}()

	membershipID := companyA.UserID

	_, err = db.Exec(ctx, `
        INSERT INTO company_memberships (user_id, company_id)
        VALUES ($1, $2)
    `, membershipID, companyA.CompanyID)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}

	defer func() {
		_, err := db.Exec(context.Background(), `
            DELETE FROM company_memberships
            WHERE user_id = $1 AND company_id = $2
        `, membershipID, companyA.CompanyID)
		if err != nil {
			t.Errorf("cleanup membership: %v", err)
		}
	}()

	repo := NewTripRepository(db)

	type fixtureTrip struct {
		id        string
		requestID string
	}

	created := make([]fixtureTrip, 0, 2)

	defer func() {
		for _, item := range created {
			if _, err := db.Exec(
				context.Background(),
				`DELETE FROM trips WHERE id = $1`,
				item.id,
			); err != nil {
				t.Errorf("cleanup trip: %v", err)
			}
			if _, err := db.Exec(
				context.Background(),
				`DELETE FROM ride_requests WHERE id = $1`,
				item.requestID,
			); err != nil {
				t.Errorf("cleanup ride request: %v", err)
			}
		}
	}()

	createTrip := func(company *testutil.DriverFixture) string {
		t.Helper()

		requestID := uuid.NewString()
		tripID := uuid.NewString()
		now := time.Now().UTC()

		_, err := db.Exec(ctx, `
            INSERT INTO ride_requests (
                id, customer_id,
                pickup_address, pickup_latitude, pickup_longitude,
                destination_address, destination_latitude,
                destination_longitude, requested_vehicle_type,
                passenger_count, status, requested_at,
                expires_at, created_at, updated_at
            )
            VALUES (
                $1, $2,
                'Tenant Test Pickup', 60.17, 24.94,
                'Tenant Test Destination', 60.18, 24.95,
                'STANDARD', 1, 'ACCEPTED',
                $3, $4, $3, $3
            )
        `, requestID, customerID, now, now.Add(time.Hour))
		if err != nil {
			t.Fatalf("create ride request: %v", err)
		}

		created = append(created, fixtureTrip{
			id: tripID, requestID: requestID,
		})

		trip := &models.Trip{
			RideRequestID: requestID,
			CustomerID:    customerID,
			DriverID:      company.UserID,
			VehicleID:     company.VehicleID,
			CompanyID:     company.CompanyID,
			BranchID:      company.BranchID,
			FleetID:       company.FleetID,
			AssignedAt:    now,
		}

		trip.ID = tripID

		if err := repo.Create(ctx, trip); err != nil {
			t.Fatalf("create trip: %v", err)
		}

		return tripID
	}

	tripA := createTrip(companyA)
	tripB := createTrip(companyB)

	list := func(companyFilter string, limit, offset int) []*models.Trip {
		t.Helper()

		trips, err := repo.ListForCompanyMember(
			ctx, membershipID,
			companyFilter, "", "", "", "",
			limit, offset,
		)
		if err != nil {
			t.Fatalf("list tenant trips: %v", err)
		}
		return trips
	}

	visible := list("", 10, 0)
	if len(visible) != 1 || visible[0].ID != tripA {
		t.Fatalf("membership must expose only company A trip: %+v", visible)
	}

	crossTenant := list(companyB.CompanyID, 10, 0)
	if len(crossTenant) != 0 {
		t.Fatalf("cross-company trip leaked: %+v", crossTenant)
	}

	paginated := list("", 1, 0)
	if len(paginated) != 1 || paginated[0].ID != tripA {
		t.Fatalf("pagination must preserve tenant isolation: %+v", paginated)
	}

	_, err = db.Exec(ctx, `
        DELETE FROM company_memberships
        WHERE user_id = $1 AND company_id = $2
    `, membershipID, companyA.CompanyID)
	if err != nil {
		t.Fatalf("revoke membership: %v", err)
	}

	revoked := list("", 10, 0)
	if len(revoked) != 0 {
		t.Fatalf("revoked member still sees trips: %+v", revoked)
	}

	_ = tripB
}
