package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	assignment "github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

func TestHandleAssignmentError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		err             error
		expectedStatus  int
		expectedMessage string
	}{
		{"missing driver", assignment.ErrDriverNotFound, http.StatusNotFound, assignment.ErrDriverNotFound.Error()},
		{"missing vehicle", assignment.ErrVehicleNotFound, http.StatusNotFound, assignment.ErrVehicleNotFound.Error()},
		{"missing assignment", assignment.ErrAssignmentNotFound, http.StatusNotFound, assignment.ErrAssignmentNotFound.Error()},
		{"inactive driver", assignment.ErrDriverInactive, http.StatusConflict, assignment.ErrDriverInactive.Error()},
		{"inactive company", assignment.ErrCompanyInactive, http.StatusConflict, assignment.ErrCompanyInactive.Error()},
		{"inactive branch", assignment.ErrBranchInactive, http.StatusConflict, assignment.ErrBranchInactive.Error()},
		{"inactive fleet", assignment.ErrFleetInactive, http.StatusConflict, assignment.ErrFleetInactive.Error()},
		{"vehicle outside driver scope", assignment.ErrVehicleOutsideDriverScope, http.StatusConflict, assignment.ErrVehicleOutsideDriverScope.Error()},
		{"inactive vehicle", assignment.ErrVehicleInactive, http.StatusConflict, assignment.ErrVehicleInactive.Error()},
		{"driver already assigned", assignment.ErrDriverAlreadyAssigned, http.StatusConflict, assignment.ErrDriverAlreadyAssigned.Error()},
		{"vehicle already assigned", assignment.ErrVehicleAlreadyAssigned, http.StatusConflict, assignment.ErrVehicleAlreadyAssigned.Error()},
		{"offline driver", assignment.ErrDriverOffline, http.StatusConflict, assignment.ErrDriverOffline.Error()},
		{"unavailable driver", assignment.ErrDriverUnavailable, http.StatusConflict, assignment.ErrDriverUnavailable.Error()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("request_id", "assignment-api-test-request")

			handleAssignmentError(c, tc.err)

			if recorder.Code != tc.expectedStatus {
				t.Fatalf(
					"expected HTTP status %d, got %d",
					tc.expectedStatus,
					recorder.Code,
				)
			}

			var body response.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}

			if body.Success {
				t.Fatal("expected success=false")
			}

			if body.Message != tc.expectedMessage {
				t.Fatalf(
					"expected message %q, got %q",
					tc.expectedMessage,
					body.Message,
				)
			}

			if body.Meta.RequestID != "assignment-api-test-request" {
				t.Fatalf(
					"expected request_id %q, got %q",
					"assignment-api-test-request",
					body.Meta.RequestID,
				)
			}
		})
	}
}

func TestHandleAssignmentErrorRecognizesWrappedDomainError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	handleAssignmentError(
		c,
		errors.Join(
			errors.New("assignment failed"),
			assignment.ErrFleetInactive,
		),
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusConflict,
			recorder.Code,
		)
	}
}

func TestHandleAssignmentErrorUnexpectedFailureReturnsInternalServerError(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("request_id", "assignment-api-internal-error-test")

	handleAssignmentError(
		c,
		errors.New("database connection details must not leak"),
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusInternalServerError,
			recorder.Code,
		)
	}

	var body response.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if body.Message != "Internal server error" {
		t.Fatalf(
			"expected sanitized internal error, got %q",
			body.Message,
		)
	}
}
