package api

import (
	"bytes"
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
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

func TestHandlePresenceError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		err             error
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "invalid latitude returns bad request",
			err:             presence.ErrInvalidLatitude,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: presence.ErrInvalidLatitude.Error(),
		},
		{
			name:            "invalid longitude returns bad request",
			err:             presence.ErrInvalidLongitude,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: presence.ErrInvalidLongitude.Error(),
		},
		{
			name:            "invalid heading returns bad request",
			err:             presence.ErrInvalidHeading,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: presence.ErrInvalidHeading.Error(),
		},
		{
			name:            "invalid speed returns bad request",
			err:             presence.ErrInvalidSpeed,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: presence.ErrInvalidSpeed.Error(),
		},
		{
			name:            "invalid accuracy returns bad request",
			err:             presence.ErrInvalidAccuracy,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: presence.ErrInvalidAccuracy.Error(),
		},
		{
			name:            "invalid availability status returns bad request",
			err:             presence.ErrInvalidAvailabilityStatus,
			expectedStatus:  http.StatusBadRequest,
			expectedMessage: presence.ErrInvalidAvailabilityStatus.Error(),
		},
		{
			name:            "missing driver returns not found",
			err:             presence.ErrDriverNotFound,
			expectedStatus:  http.StatusNotFound,
			expectedMessage: presence.ErrDriverNotFound.Error(),
		},
		{
			name:            "missing assignment returns conflict",
			err:             presence.ErrDriverAssignmentRequired,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrDriverAssignmentRequired.Error(),
		},
		{
			name:            "availability locked returns conflict",
			err:             presence.ErrDriverAvailabilityLocked,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrDriverAvailabilityLocked.Error(),
		},
		{
			name:            "offline heartbeat returns conflict",
			err:             presence.ErrDriverHeartbeatUnavailable,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrDriverHeartbeatUnavailable.Error(),
		},
		{
			name:            "regulatory compliance required returns conflict",
			err:             presence.ErrDriverComplianceRequired,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrDriverComplianceRequired.Error(),
		},
		{
			name:            "offline driver returns conflict",
			err:             presence.ErrDriverOffline,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrDriverOffline.Error(),
		},
		{
			name:            "inactive driver returns conflict",
			err:             presence.ErrDriverInactive,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrDriverInactive.Error(),
		},
		{
			name:            "inactive company returns conflict",
			err:             presence.ErrCompanyInactive,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrCompanyInactive.Error(),
		},
		{
			name:            "inactive branch returns conflict",
			err:             presence.ErrBranchInactive,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrBranchInactive.Error(),
		},
		{
			name:            "inactive fleet returns conflict",
			err:             presence.ErrFleetInactive,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrFleetInactive.Error(),
		},
		{
			name:            "inactive vehicle returns conflict",
			err:             presence.ErrVehicleInactive,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrVehicleInactive.Error(),
		},
		{
			name:            "invalid assignment scope returns conflict",
			err:             presence.ErrAssignmentScopeInvalid,
			expectedStatus:  http.StatusConflict,
			expectedMessage: presence.ErrAssignmentScopeInvalid.Error(),
		},
	}

	for _, testCase := range tests {
		t.Run(
			testCase.name,
			func(t *testing.T) {
				recorder := httptest.NewRecorder()

				c, _ := gin.CreateTestContext(recorder)

				c.Set(
					"request_id",
					"presence-api-test-request",
				)

				handlePresenceError(
					c,
					testCase.err,
				)

				if recorder.Code != testCase.expectedStatus {
					t.Fatalf(
						"expected HTTP status %d, got %d",
						testCase.expectedStatus,
						recorder.Code,
					)
				}

				var body response.ErrorResponse

				if err := json.Unmarshal(
					recorder.Body.Bytes(),
					&body,
				); err != nil {
					t.Fatalf(
						"decode error response: %v",
						err,
					)
				}

				if body.Success {
					t.Fatal(
						"expected success=false",
					)
				}

				if body.Message != testCase.expectedMessage {
					t.Fatalf(
						"expected message %q, got %q",
						testCase.expectedMessage,
						body.Message,
					)
				}

				if body.Meta.RequestID !=
					"presence-api-test-request" {

					t.Fatalf(
						"expected request_id %q, got %q",
						"presence-api-test-request",
						body.Meta.RequestID,
					)
				}
			},
		)
	}
}

func TestHandlePresenceErrorUnexpectedFailureReturnsInternalServerError(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Set(
		"request_id",
		"presence-api-internal-error-test",
	)

	handlePresenceError(
		c,
		errUnexpectedPresenceFailure{},
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusInternalServerError,
			recorder.Code,
		)
	}

	var body response.ErrorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode internal error response: %v",
			err,
		)
	}

	if body.Success {
		t.Fatal(
			"expected success=false",
		)
	}

	if body.Message != "Internal server error" {
		t.Fatalf(
			"expected generic internal error message, got %q",
			body.Message,
		)
	}
}

type errUnexpectedPresenceFailure struct{}

func (errUnexpectedPresenceFailure) Error() string {
	return "unexpected database failure"
}

func TestListAvailableDriverNotFoundMapsToNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Set(
		"request_id",
		"presence-list-available-not-found",
	)

	handlePresenceError(
		c,
		presence.ErrDriverNotFound,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusNotFound,
			recorder.Code,
		)
	}

	var body response.ErrorResponse

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatalf(
			"decode list-available not-found response: %v",
			err,
		)
	}

	if body.Success {
		t.Fatal(
			"expected success=false",
		)
	}

	if body.Message != presence.ErrDriverNotFound.Error() {
		t.Fatalf(
			"expected message %q, got %q",
			presence.ErrDriverNotFound.Error(),
			body.Message,
		)
	}

	if body.Meta.RequestID !=
		"presence-list-available-not-found" {

		t.Fatalf(
			"expected request_id %q, got %q",
			"presence-list-available-not-found",
			body.Meta.RequestID,
		)
	}
}

// A deterministic regulatory rejection exercises the real presence service
// and HTTP handler without relying on document expiry dates.
type rejectingPresenceCompliance struct{}

func (rejectingPresenceCompliance) IsEligible(
	_ context.Context,
	_ string,
	_ time.Time,
) (bool, error) {
	return false, nil
}

func TestUpdateAvailabilityHTTPRegulatoryRejectionReturnsConflict(
	t *testing.T,
) {
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

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`INSERT INTO driver_presence (
            driver_id, company_id, branch_id, assignment_id,
            vehicle_id, is_online, availability_status, updated_at
        ) VALUES ($1, $2, $3, $4, $5, TRUE, 'BREAK', NOW())`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
	)
	if err != nil {
		t.Fatalf("create online break presence: %v", err)
	}
	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		); err != nil {
			t.Errorf("cleanup driver presence: %v", err)
		}
	}()

	service := presence.NewService(presence.Dependencies{
		DB:          db,
		Config:      cfg,
		Drivers:     postgresrepo.NewDriverRepository(db),
		Presence:    postgresrepo.NewDriverPresenceRepository(db),
		Assignments: postgresrepo.NewDriverAssignmentRepository(db),
		Compliance:  rejectingPresenceCompliance{},
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PATCH(
		"/api/v1/driver/availability",
		func(c *gin.Context) {
			middleware.SetCurrentUser(
				c,
				&models.User{
					BaseModel: models.BaseModel{ID: fixture.UserID},
				},
			)
			c.Next()
		},
		NewDriverPresenceHandler(service).UpdateAvailability,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/driver/availability",
		bytes.NewBufferString(`{"status":"AVAILABLE"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected HTTP 409 for regulatory rejection, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body response.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Success {
		t.Fatal("expected success=false")
	}
	if body.Message != presence.ErrDriverComplianceRequired.Error() {
		t.Fatalf(
			"expected compliance message %q, got %q",
			presence.ErrDriverComplianceRequired.Error(),
			body.Message,
		)
	}

	var isOnline bool
	var availability string
	if err := db.QueryRow(
		ctx,
		`SELECT is_online, availability_status
         FROM driver_presence WHERE driver_id = $1`,
		fixture.UserID,
	).Scan(&isOnline, &availability); err != nil {
		t.Fatalf("read presence after regulatory rejection: %v", err)
	}

	if !isOnline || availability != "BREAK" {
		t.Fatalf(
			"regulatory rejection changed presence: online=%v availability=%s",
			isOnline,
			availability,
		)
	}
}
