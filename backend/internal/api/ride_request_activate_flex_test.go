package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestActivateFlexHTTPRejectsUnauthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewRideRequestHandler(nil)

	router.POST(
		"/api/v1/ride-requests/:id/activate-flex",
		handler.ActivateFlex,
	)

	rideID := uuid.NewString()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ride-requests/"+rideID+"/activate-flex",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP 401, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var response struct {
		Success bool `json:"success"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if response.Success {
		t.Fatal("unauthenticated activation unexpectedly succeeded")
	}
}

func TestActivateFlexHTTPRejectsMalformedRideID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewRideRequestHandler(nil)

	router.Use(func(c *gin.Context) {
		user := &models.User{}
		user.ID = uuid.NewString()

		c.Set(
			middleware.ContextUserKey,
			user,
		)
		c.Next()
	})

	router.POST(
		"/api/v1/ride-requests/:id/activate-flex",
		handler.ActivateFlex,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ride-requests/not-a-uuid/activate-flex",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP 400, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var response struct {
		Success bool `json:"success"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	if response.Success {
		t.Fatal("malformed ride ID unexpectedly succeeded")
	}
}

// TestActivateFlexRouteRequiresAuthentication verifies that the production
// route remains inside the authenticated ride-request group.
func TestActivateFlexRouteRequiresAuthentication(t *testing.T) {
	sourceBytes, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}

	source := string(sourceBytes)

	start := strings.Index(
		source,
		`rideRequests := v1.Group("/ride-requests")`,
	)
	if start < 0 {
		t.Fatal("ride-request route group missing")
	}

	end := strings.Index(
		source[start:],
		`func registerPaymentCallbackRoutes(`,
	)
	if end < 0 {
		t.Fatal("ride-request route section boundary missing")
	}

	section := strings.Join(
		strings.Fields(source[start:start+end]),
		"",
	)

	for _, expected := range []string{
		`rideRequests.Use(authMiddleware.RequireAuth())`,
		`rideRequests.POST("/:id/activate-flex",rideRequestHandler.ActivateFlex,)`,
	} {
		normalized := strings.Join(strings.Fields(expected), "")
		if !strings.Contains(section, normalized) {
			t.Fatalf(
				"missing FLEX route authentication requirement: %s",
				expected,
			)
		}
	}
}

// TestActivateFlexHTTPMiddlewareRejectsMissingToken exercises CONNECT's
// actual RequireAuth middleware, rather than calling the handler directly.
func TestActivateFlexHTTPMiddlewareRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	authMiddleware := middleware.NewAuthMiddleware(nil, nil)

	rideRequests := router.Group("/api/v1/ride-requests")
	rideRequests.Use(authMiddleware.RequireAuth())

	handlerReached := false

	rideRequests.POST(
		"/:id/activate-flex",
		func(c *gin.Context) {
			handlerReached = true
			c.Status(http.StatusNoContent)
		},
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ride-requests/"+uuid.NewString()+"/activate-flex",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP 401, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if handlerReached {
		t.Fatal("unauthenticated request reached FLEX activation handler")
	}
}
