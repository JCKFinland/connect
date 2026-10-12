package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDispatchOfferAcceptRouteRequiresAuthenticationAndDriverPermission(t *testing.T) {
	sourceBytes, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	source := string(sourceBytes)
	start := strings.Index(source, `drivers := v1.Group("/drivers")`)
	if start < 0 {
		t.Fatal("drivers route group missing")
	}
	end := strings.Index(source[start:], `// Driver Regulatory Documents`)
	if end < 0 {
		t.Fatal("dispatch offer route section boundary missing")
	}
	section := source[start : start+end]
	for _, expected := range []string{
		`drivers.Use(authMiddleware.RequireAuth())`,
		`drivers.POST(
				"/dispatch-offers/:offer_id/accept",
				rbacMiddleware.RequirePermission("driver.operations"),
				dispatchHandler.AcceptOffer,
			)`,
	} {
		if !strings.Contains(
			strings.Join(strings.Fields(section), ""),
			strings.Join(strings.Fields(expected), ""),
		) {
			t.Errorf("missing required dispatch accept route guard:\n%s", expected)
		}
	}
}

func TestDispatchOfferAcceptRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/drivers/dispatch-offers/offer-123/accept", nil)
	context.Params = gin.Params{{Key: "offer_id", Value: "offer-123"}}
	handler := NewDispatchHandler(nil)
	handler.AcceptOffer(context)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without authenticated user, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
