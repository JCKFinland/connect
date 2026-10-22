package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const passengerAuthorizationRoutePath = "/api/v1/ride-requests/:id/flex-auctions/:auction_id/authorize"

type routeAuthorizationExecutor struct {
	calls int
}

func (e *routeAuthorizationExecutor) Execute(
	_ context.Context,
	_, _, _ string,
) (*flexauthorization.ExecuteResult, error) {
	e.calls++
	return &flexauthorization.ExecuteResult{
		Status:     "AUTHORIZED",
		Authorized: true,
	}, nil
}

func newPassengerAuthorizationProductionTestRouter(
	handler *PassengerFlexAuthorizationHandler,
) *gin.Engine {
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
		handler,
	)

	return router
}

func passengerAuthorizationRouteRegistered(
	router *gin.Engine,
) bool {
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost &&
			route.Path == passengerAuthorizationRoutePath {
			return true
		}
	}
	return false
}

func TestPassengerFlexAuthorizationProductionRouteRegistration(
	t *testing.T,
) {
	t.Run("disabled without handler", func(t *testing.T) {
		router := newPassengerAuthorizationProductionTestRouter(nil)

		if passengerAuthorizationRouteRegistered(router) {
			t.Fatal(
				"authorization route registered without service",
			)
		}
	})

	t.Run("enabled with handler", func(t *testing.T) {
		executor := &routeAuthorizationExecutor{}

		handler := NewPassengerFlexAuthorizationHandler(
			executor,
		)

		router := newPassengerAuthorizationProductionTestRouter(
			handler,
		)

		if !passengerAuthorizationRouteRegistered(router) {
			t.Fatal(
				"authorization route missing with initialized handler",
			)
		}

		if executor.calls != 0 {
			t.Fatal(
				"authorization executed during route registration",
			)
		}
	})

	t.Run("JWT required before authorization", func(t *testing.T) {
		executor := &routeAuthorizationExecutor{}

		handler := NewPassengerFlexAuthorizationHandler(
			executor,
		)

		router := newPassengerAuthorizationProductionTestRouter(
			handler,
		)

		rideID := uuid.NewString()
		auctionID := uuid.NewString()

		path := "/api/v1/ride-requests/" +
			rideID +
			"/flex-auctions/" +
			auctionID +
			"/authorize"

		request := httptest.NewRequest(
			http.MethodPost,
			path,
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

		if executor.calls != 0 {
			t.Fatal(
				"authorization executed without JWT authentication",
			)
		}
	})
}
