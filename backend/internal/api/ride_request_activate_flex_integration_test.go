package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupFlexHTTPIntegration(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	var customerID string
	err = db.QueryRow(
		context.Background(),
		`
		SELECT u.id
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles r ON r.id = ur.role_id
		WHERE u.deleted_at IS NULL
		GROUP BY u.id
		HAVING BOOL_OR(r.name = 'CUSTOMER')
		   AND COUNT(*) FILTER (WHERE r.name <> 'CUSTOMER') = 0
		ORDER BY u.id
		LIMIT 1
		`,
	).Scan(&customerID)
	if err != nil {
		t.Fatalf("resolve CUSTOMER fixture: %v", err)
	}

	return db, customerID
}

func insertFlexHTTPRide(
	t *testing.T,
	db *pgxpool.Pool,
	customerID string,
) string {
	t.Helper()

	rideID := uuid.NewString()
	now := time.Now().UTC()

	_, err := db.Exec(
		context.Background(),
		`
		INSERT INTO ride_requests (
			id, customer_id,
			pickup_address, pickup_latitude, pickup_longitude,
			destination_address, destination_latitude,
			destination_longitude,
			requested_vehicle_type, passenger_count,
			status, matching_mode, notes,
			requested_at, expires_at, created_at, updated_at
		)
		VALUES (
			$1, $2,
			'FLEX HTTP Pickup', 60.2055, 24.6559,
			'FLEX HTTP Destination', 60.1719, 24.9414,
			'STANDARD', 1,
			'PENDING', 'STANDARD', 'FLEX HTTP integration test',
			$3, $4, $3, $3
		)
		`,
		rideID,
		customerID,
		now,
		now.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("insert FLEX HTTP ride: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideID,
		)
		if err != nil {
			t.Errorf("cleanup FLEX HTTP ride: %v", err)
		}
	})

	return rideID
}

func executeFlexHTTP(
	t *testing.T,
	db *pgxpool.Pool,
	rideID string,
	userID string,
) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)

	service := ride_request.NewService(ride_request.Dependencies{
		DB:        db,
		UserRoles: repository.NewUserRoleRepository(db),
	})

	router := gin.New()

	router.POST(
		"/api/v1/ride-requests/:id/activate-flex",
		func(c *gin.Context) {
			middleware.SetCurrentUser(
				c,
				&models.User{
					BaseModel: models.BaseModel{ID: userID},
				},
			)
			c.Next()
		},
		NewRideRequestHandler(service).ActivateFlex,
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/ride-requests/"+rideID+"/activate-flex",
			nil,
		),
	)

	return recorder
}

func TestActivateFlexHTTPIntegrationSucceeds(t *testing.T) {
	db, customerID := setupFlexHTTPIntegration(t)
	rideID := insertFlexHTTPRide(t, db, customerID)

	var beforeUpdatedAt time.Time

	err := db.QueryRow(
		context.Background(),
		`SELECT updated_at FROM ride_requests WHERE id = $1`,
		rideID,
	).Scan(&beforeUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}

	recorder := executeFlexHTTP(t, db, rideID, customerID)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			RideRequestID string `json:"ride_request_id"`
			MatchingMode  string `json:"matching_mode"`
		} `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if !response.Success ||
		response.Data.RideRequestID != rideID ||
		response.Data.MatchingMode != "FLEX" {
		t.Fatalf("unexpected success response: %s", recorder.Body.String())
	}

	var status, matchingMode string
	var marker *time.Time
	var afterUpdatedAt time.Time

	err = db.QueryRow(
		context.Background(),
		`
		SELECT status, matching_mode,
		       standard_dispatch_started_at, updated_at
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(
		&status,
		&matchingMode,
		&marker,
		&afterUpdatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" ||
		matchingMode != "FLEX" ||
		marker != nil ||
		!beforeUpdatedAt.Equal(afterUpdatedAt) {
		t.Fatalf(
			"unexpected persisted state: status=%s mode=%s marker=%v updated_at=%v",
			status,
			matchingMode,
			marker,
			afterUpdatedAt,
		)
	}
}

func TestActivateFlexHTTPIntegrationRejectsNonOwner(t *testing.T) {
	db, customerID := setupFlexHTTPIntegration(t)
	rideID := insertFlexHTTPRide(t, db, customerID)

	recorder := executeFlexHTTP(
		t,
		db,
		rideID,
		uuid.NewString(),
	)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf(
			"expected HTTP 403, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var mode string
	if err := db.QueryRow(
		context.Background(),
		`SELECT matching_mode FROM ride_requests WHERE id = $1`,
		rideID,
	).Scan(&mode); err != nil {
		t.Fatal(err)
	}

	if mode != "STANDARD" {
		t.Fatalf("non-owner changed matching mode to %s", mode)
	}
}

func TestActivateFlexHTTPIntegrationRejectsDispatchHistory(t *testing.T) {
	db, customerID := setupFlexHTTPIntegration(t)
	rideID := insertFlexHTTPRide(t, db, customerID)

	_, err := db.Exec(
		context.Background(),
		`
		UPDATE ride_requests
		SET standard_dispatch_started_at = NOW()
		WHERE id = $1
		`,
		rideID,
	)
	if err != nil {
		t.Fatal(err)
	}

	recorder := executeFlexHTTP(t, db, rideID, customerID)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected HTTP 409, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var mode string
	var marker *time.Time

	err = db.QueryRow(
		context.Background(),
		`
		SELECT matching_mode, standard_dispatch_started_at
		FROM ride_requests
		WHERE id = $1
		`,
		rideID,
	).Scan(&mode, &marker)
	if err != nil {
		t.Fatal(err)
	}

	if mode != "STANDARD" || marker == nil {
		t.Fatalf(
			"dispatch history changed: mode=%s marker=%v",
			mode,
			marker,
		)
	}
}

func TestActivateFlexHTTPIntegrationReturnsNotFound(t *testing.T) {
	db, customerID := setupFlexHTTPIntegration(t)

	recorder := executeFlexHTTP(
		t,
		db,
		uuid.NewString(),
		customerID,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP 404, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

// TestActivateFlexHTTPIntegrationRejectsIneligibleLifecycle verifies that
// FLEX activation cannot bypass ride-request lifecycle restrictions.
func TestActivateFlexHTTPIntegrationRejectsIneligibleLifecycle(
	t *testing.T,
) {
	db, customerID := setupFlexHTTPIntegration(t)

	tests := []struct {
		name         string
		status       string
		matchingMode string
		expired      bool
	}{
		{
			name:         "expired",
			status:       "PENDING",
			matchingMode: "STANDARD",
			expired:      true,
		},
		{
			name:         "cancelled",
			status:       "CANCELLED",
			matchingMode: "STANDARD",
		},
		{
			name:         "matching",
			status:       "MATCHING",
			matchingMode: "STANDARD",
		},
		{
			name:         "accepted",
			status:       "ACCEPTED",
			matchingMode: "STANDARD",
		},
		{
			name:         "already_flex",
			status:       "PENDING",
			matchingMode: "FLEX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rideID := insertFlexHTTPRide(t, db, customerID)

			expiresAt := time.Now().UTC().Add(time.Hour)
			if tt.expired {
				expiresAt = time.Now().UTC().Add(-time.Hour)
			}

			_, err := db.Exec(
				context.Background(),
				`
				UPDATE ride_requests
				SET status = $2,
				    matching_mode = $3,
				    expires_at = $4
				WHERE id = $1
				`,
				rideID,
				tt.status,
				tt.matchingMode,
				expiresAt,
			)
			if err != nil {
				t.Fatalf("prepare lifecycle fixture: %v", err)
			}

			recorder := executeFlexHTTP(
				t,
				db,
				rideID,
				customerID,
			)

			if recorder.Code != http.StatusConflict {
				t.Fatalf(
					"expected HTTP 409, got %d: %s",
					recorder.Code,
					recorder.Body.String(),
				)
			}

			var status, matchingMode string
			var marker *time.Time

			err = db.QueryRow(
				context.Background(),
				`
				SELECT status, matching_mode,
				       standard_dispatch_started_at
				FROM ride_requests
				WHERE id = $1
				`,
				rideID,
			).Scan(&status, &matchingMode, &marker)
			if err != nil {
				t.Fatal(err)
			}

			if status != tt.status ||
				matchingMode != tt.matchingMode ||
				marker != nil {
				t.Fatalf(
					"rejected activation changed lifecycle: "+
						"status=%s mode=%s marker=%v",
					status,
					matchingMode,
					marker,
				)
			}
		})
	}
}
