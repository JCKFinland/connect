package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/dispatch"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Tests the real HTTP handler and PostgreSQL-backed ownership decision.
// Auth middleware JWT/RBAC wiring is covered separately by route-boundary tests.
func TestDispatchOfferAcceptHTTPRejectsOtherDriverOwnership(t *testing.T) {
	ctx := context.Background()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldDir) }()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, cleanupOwner, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupOwner(context.Background()); err != nil {
			t.Errorf("cleanup owner: %v", err)
		}
	}()
	intruder, cleanupIntruder, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupIntruder(context.Background()); err != nil {
			t.Errorf("cleanup intruder: %v", err)
		}
	}()
	categoryID, rideID, offerID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	// Delete dependent rows before fixture cleanup; no existing rows are modified.
	defer func() {
		cleanup := context.Background()
		for _, item := range []struct {
			query string
			id    string
		}{
			{"DELETE FROM trips WHERE ride_request_id=$1", rideID},
			{"DELETE FROM dispatch_offers WHERE id=$1", offerID},
			{"DELETE FROM ride_requests WHERE id=$1", rideID},
			{"DELETE FROM service_categories WHERE id=$1", categoryID},
		} {
			if _, err := db.Exec(cleanup, item.query, item.id); err != nil {
				t.Errorf("cleanup %s: %v", item.id, err)
			}
		}
	}()
	_, err = db.Exec(ctx, `INSERT INTO service_categories (id,code,name,is_active,created_at,updated_at) VALUES ($1,$2,'HTTP Ownership Test',TRUE,NOW(),NOW())`, categoryID, "HTTP_OWNER_"+uuid.NewString()[:8])
	if err != nil {
		t.Fatal(err)
	}
	// The request and offer are real PostgreSQL rows. Authorization must reject
	// the non-owner before any trip-acceptance side effects occur.
	_, err = db.Exec(ctx, `INSERT INTO ride_requests (id,customer_id,pickup_address,pickup_latitude,pickup_longitude,destination_address,destination_latitude,destination_longitude,requested_vehicle_type,service_category_id,passenger_count,status,requested_at,expires_at,created_at,updated_at) VALUES ($1,$2,'Test pickup',60.2,24.6,'Test destination',60.3,24.7,'STANDARD',$3,1,'MATCHING',$4,$5,$4,$4)`, rideID, "49c61249-8b7d-4afd-a559-6d54567ee164", categoryID, now, now.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO dispatch_offers (id,ride_request_id,driver_id,vehicle_id,company_id,branch_id,fleet_id,assignment_id,status,offered_at,expires_at,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',$9,$10,$9,$9)`, offerID, rideID, owner.DriverID, owner.VehicleID, owner.CompanyID, owner.BranchID, owner.FleetID, owner.AssignmentID, now, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	service := dispatch.NewService(dispatch.Dependencies{DB: db, Config: cfg, Drivers: postgresrepo.NewDriverRepository(db), Offers: postgresrepo.NewDispatchOfferRepository(db)})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/drivers/dispatch-offers/:offer_id/accept", func(c *gin.Context) {
		middleware.SetCurrentUser(c, &models.User{BaseModel: models.BaseModel{ID: intruder.UserID}})
		c.Next()
	}, NewDispatchHandler(service).AcceptOffer)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/drivers/dispatch-offers/"+offerID+"/accept", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var status string
	if err := db.QueryRow(ctx, "SELECT status FROM dispatch_offers WHERE id=$1", offerID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "PENDING" {
		t.Fatalf("unauthorized request changed offer status to %s", status)
	}
	var trips int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM trips WHERE ride_request_id=$1", rideID).Scan(&trips); err != nil {
		t.Fatal(err)
	}
	if trips != 0 {
		t.Fatalf("unauthorized request created %d trips", trips)
	}
}
