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

const flexAuctionTestRoute = "/api/v1/ride-requests/:id/flex-auctions"

func newFlexAuctionTestRouter(authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	if authenticated {
		router.Use(func(c *gin.Context) {
			user := &models.User{}
			user.ID = uuid.NewString()
			middleware.SetCurrentUser(c, user)
			c.Next()
		})
	}

	handler := NewRideRequestHandler(nil)

	router.POST(
		flexAuctionTestRoute,
		handler.InitiateFlexAuction,
	)

	return router
}

func executeFlexAuctionRequest(
	router *gin.Engine,
	rideID string,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ride-requests/"+rideID+"/flex-auctions",
		strings.NewReader(body),
	)

	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func assertFlexAuctionHTTPFailure(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	expectedStatus int,
) {
	t.Helper()

	if recorder.Code != expectedStatus {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			expectedStatus,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var result struct {
		Success bool `json:"success"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&result,
	); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}

	if result.Success {
		t.Fatal("rejected FLEX auction request unexpectedly succeeded")
	}
}

func TestInitiateFlexAuctionHTTPRejectsUnauthenticatedUser(
	t *testing.T,
) {
	router := newFlexAuctionTestRouter(false)

	recorder := executeFlexAuctionRequest(
		router,
		uuid.NewString(),
		`{"passenger_offer_cents":2328}`,
	)

	assertFlexAuctionHTTPFailure(
		t,
		recorder,
		http.StatusUnauthorized,
	)
}

func TestInitiateFlexAuctionHTTPRejectsMalformedRideID(
	t *testing.T,
) {
	router := newFlexAuctionTestRouter(true)

	recorder := executeFlexAuctionRequest(
		router,
		"not-a-uuid",
		`{"passenger_offer_cents":2328}`,
	)

	assertFlexAuctionHTTPFailure(
		t,
		recorder,
		http.StatusBadRequest,
	)
}

func TestInitiateFlexAuctionHTTPRejectsInvalidOffers(
	t *testing.T,
) {
	testCases := []struct {
		name string
		body string
	}{
		{
			name: "missing offer",
			body: `{}`,
		},
		{
			name: "zero offer",
			body: `{"passenger_offer_cents":0}`,
		},
		{
			name: "negative offer",
			body: `{"passenger_offer_cents":-1}`,
		},
		{
			name: "fractional cents",
			body: `{"passenger_offer_cents":23.28}`,
		},
		{
			name: "string amount",
			body: `{"passenger_offer_cents":"2328"}`,
		},
		{
			name: "malformed JSON",
			body: `{"passenger_offer_cents":`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := newFlexAuctionTestRouter(true)

			recorder := executeFlexAuctionRequest(
				router,
				uuid.NewString(),
				tc.body,
			)

			assertFlexAuctionHTTPFailure(
				t,
				recorder,
				http.StatusBadRequest,
			)
		})
	}
}

func TestInitiateFlexAuctionRouteRequiresAuthentication(
	t *testing.T,
) {
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
		`rideRequests.POST("/:id/flex-auctions",rideRequestHandler.InitiateFlexAuction,)`,
	} {
		normalized := strings.Join(
			strings.Fields(expected),
			"",
		)

		if !strings.Contains(section, normalized) {
			t.Fatalf(
				"missing authenticated FLEX auction route: %s",
				expected,
			)
		}
	}
}

func TestInitiateFlexAuctionMiddlewareRejectsMissingToken(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	authMiddleware := middleware.NewAuthMiddleware(nil, nil)

	rideRequests := router.Group("/api/v1/ride-requests")
	rideRequests.Use(authMiddleware.RequireAuth())

	handlerReached := false

	rideRequests.POST(
		"/:id/flex-auctions",
		func(c *gin.Context) {
			handlerReached = true
			c.Status(http.StatusNoContent)
		},
	)

	recorder := executeFlexAuctionRequest(
		router,
		uuid.NewString(),
		`{"passenger_offer_cents":2328}`,
	)

	assertFlexAuctionHTTPFailure(
		t,
		recorder,
		http.StatusUnauthorized,
	)

	if handlerReached {
		t.Fatal(
			"unauthenticated request reached FLEX auction handler",
		)
	}
}
