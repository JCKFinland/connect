package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const passengerConfirmationRoutePath = "/api/v1/ride-requests/:id/flex-auctions/:auction_id/confirm"

func passengerConfirmationRouteRegistered(
	router *gin.Engine,
) bool {
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost &&
			route.Path == passengerConfirmationRoutePath {
			return true
		}
	}
	return false
}

func TestPassengerFlexConfirmationProductionRouteRegistration(
	t *testing.T,
) {
	t.Run("disabled without authorization handler", func(t *testing.T) {
		router := newPassengerAuthorizationProductionTestRouter(nil)

		if passengerConfirmationRouteRegistered(router) {
			t.Fatal("confirmation route registered without handler")
		}
	})

	t.Run("disabled without confirmation handler", func(t *testing.T) {
		authorization := NewPassengerFlexAuthorizationHandler(
			&routeAuthorizationExecutor{},
		)

		router := newPassengerAuthorizationProductionTestRouter(
			authorization,
		)

		if passengerConfirmationRouteRegistered(router) {
			t.Fatal("confirmation route registered without confirmation service")
		}
	})

	t.Run("enabled with confirmation handler", func(t *testing.T) {
		authorization := NewPassengerFlexAuthorizationHandler(
			&routeAuthorizationExecutor{},
		)

		confirmation := NewPassengerFlexConfirmationHandler(
			&fakePassengerFlexRideConfirmer{},
		)

		authorization.SetConfirmationHandler(confirmation)

		router := newPassengerAuthorizationProductionTestRouter(
			authorization,
		)

		if !passengerConfirmationRouteRegistered(router) {
			t.Fatal("confirmation route missing")
		}

		if !passengerAuthorizationRouteRegistered(router) {
			t.Fatal("authorization route unexpectedly missing")
		}
	})

	t.Run("JWT required before confirmation", func(t *testing.T) {
		authorization := NewPassengerFlexAuthorizationHandler(
			&routeAuthorizationExecutor{},
		)

		confirmer := &fakePassengerFlexRideConfirmer{}

		authorization.SetConfirmationHandler(
			NewPassengerFlexConfirmationHandler(confirmer),
		)

		router := newPassengerAuthorizationProductionTestRouter(
			authorization,
		)

		path := "/api/v1/ride-requests/" +
			uuid.NewString() +
			"/flex-auctions/" +
			uuid.NewString() +
			"/confirm"

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

		if confirmer.calls != 0 {
			t.Fatal("confirmation executed without JWT")
		}
	})
}
