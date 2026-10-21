package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestFlexSettlementHealthProductionRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	RegisterRoutes(
		router,
		nil,
		nil,
		nil,
		middleware.NewAuthMiddleware(nil, nil),
		middleware.NewRBACMiddleware(nil),
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
	)

	found := false

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet &&
			route.Path == settlementHealthTestPath {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf(
			"missing production route: GET %s",
			settlementHealthTestPath,
		)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		settlementHealthTestPath,
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
}
