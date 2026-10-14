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

const flexDriverOfferTestRoute = "/api/v1/drivers/flex-auctions/:auction_id/offers/accept"

func newFlexDriverOfferTestRouter(
	authenticated bool,
) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	if authenticated {
		router.Use(func(c *gin.Context) {
			middleware.SetCurrentUser(
				c,
				&models.User{
					BaseModel: models.BaseModel{
						ID: uuid.NewString(),
					},
				},
			)
			c.Next()
		})
	}

	handler := &FlexOfferHandler{}

	router.POST(
		flexDriverOfferTestRoute,
		handler.SubmitAcceptFare,
	)

	return router
}

func executeFlexDriverOfferRequest(
	router *gin.Engine,
	auctionID string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/drivers/flex-auctions/"+
			auctionID+"/offers/accept",
		nil,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func assertFlexDriverOfferHTTPFailure(
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
		t.Fatal("rejected FLEX driver offer unexpectedly succeeded")
	}
}

func TestFlexDriverOfferHTTPRejectsMissingAuthenticatedUser(
	t *testing.T,
) {
	router := newFlexDriverOfferTestRouter(false)

	recorder := executeFlexDriverOfferRequest(
		router,
		uuid.NewString(),
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusUnauthorized,
	)
}

func TestFlexDriverOfferHTTPRejectsMalformedAuctionID(
	t *testing.T,
) {
	router := newFlexDriverOfferTestRouter(true)

	recorder := executeFlexDriverOfferRequest(
		router,
		"not-a-uuid",
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusBadRequest,
	)
}

func TestFlexDriverOfferRouteRequiresAuthenticationAndPermission(
	t *testing.T,
) {
	sourceBytes, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}

	source := strings.Join(
		strings.Fields(string(sourceBytes)),
		"",
	)

	required := []string{
		`drivers:=v1.Group("/drivers")`,
		`drivers.Use(authMiddleware.RequireAuth())`,
		`drivers.POST("/flex-auctions/:auction_id/offers/accept",rbacMiddleware.RequirePermission("driver.operations"),flexOfferHandler.SubmitAcceptFare,)`,
	}

	for _, expected := range required {
		if !strings.Contains(source, expected) {
			t.Fatalf(
				"missing FLEX driver offer security contract: %s",
				expected,
			)
		}
	}
}
