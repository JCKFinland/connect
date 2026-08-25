package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
