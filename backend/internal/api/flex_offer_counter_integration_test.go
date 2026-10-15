package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const flexCounterOfferTestRoute = "/api/v1/drivers/flex-auctions/:auction_id/offers/counter"

func newFlexCounterOfferIntegrationRouter(
	f *testutil.FlexAcceptFixture,
) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	router.Use(func(c *gin.Context) {
		middleware.SetCurrentUser(
			c,
			&models.User{
				BaseModel: models.BaseModel{
					ID: f.DriverID,
				},
			},
		)
		c.Next()
	})

	handler := NewFlexOfferHandler(f.DB, f.Config)

	router.POST(
		flexCounterOfferTestRoute,
		handler.SubmitCounterOffer,
	)

	return router
}

func executeFlexCounterOfferRequest(
	router *gin.Engine,
	auctionID string,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/drivers/flex-auctions/"+
			auctionID+"/offers/counter",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestFlexCounterOfferHTTPRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"missing_amount", `{}`},
		{"null_amount", `{"proposed_amount_cents":null}`},
		{"zero", `{"proposed_amount_cents":0}`},
		{"negative", `{"proposed_amount_cents":-100}`},
		{"fractional", `{"proposed_amount_cents":2500.5}`},
		{"string_amount", `{"proposed_amount_cents":"2500"}`},
		{"unknown_field", `{"proposed_amount_cents":2500,"driver_id":"` + uuid.NewString() + `"}`},
		{"multiple_objects", `{"proposed_amount_cents":2500}{"proposed_amount_cents":2501}`},
		{"array", `[{"proposed_amount_cents":2500}]`},
		{"below_minimum", `{"proposed_amount_cents":2094}`},
		{"above_maximum", `{"proposed_amount_cents":2562}`},
		{"unchanged_passenger_fare", `{"proposed_amount_cents":2328}`},
		{"oversized", `{"proposed_amount_cents":2500,"padding":"` +
			strings.Repeat("x", 4096) + `"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.SetupFlexAcceptFixture(t)
			router := newFlexCounterOfferIntegrationRouter(f)

			recorder := executeFlexCounterOfferRequest(
				router,
				f.AuctionID,
				tc.body,
			)

			assertFlexDriverOfferHTTPFailure(
				t,
				recorder,
				http.StatusBadRequest,
			)

			assertFlexHTTPNoPersistedOffers(t, f)
		})
	}
}

func TestFlexCounterOfferHTTPRejectsMalformedAuctionID(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router := newFlexCounterOfferIntegrationRouter(f)

	recorder := executeFlexCounterOfferRequest(
		router,
		"not-a-uuid",
		`{"proposed_amount_cents":2500}`,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusBadRequest,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexCounterOfferHTTPIntegrationSucceeds(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router := newFlexCounterOfferIntegrationRouter(f)

	recorder := executeFlexCounterOfferRequest(
		router,
		f.AuctionID,
		`{"proposed_amount_cents":2500}`,
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected HTTP 201, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var result struct {
		Success bool             `json:"success"`
		Data    models.FlexOffer `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&result,
	); err != nil {
		t.Fatalf("decode counter-offer response: %v", err)
	}

	offer := result.Data

	if !result.Success ||
		offer.ID == "" ||
		offer.AuctionID != f.AuctionID ||
		offer.DriverID != f.DriverID ||
		offer.AssignmentID != f.AssignmentID ||
		offer.OfferType != models.FlexOfferTypeCounterOffer ||
		offer.OfferedAmountCents != 2500 ||
		offer.Status != models.FlexOfferStatusPending {
		t.Fatalf("unexpected counter-offer response: %+v", result)
	}

	var count int

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT COUNT(*)
         FROM flex_offers
         WHERE id = $1
           AND auction_id = $2
           AND driver_id = $3
           AND assignment_id = $4
           AND offer_type = 'COUNTER_OFFER'
           AND offered_amount = 25.00
           AND status = 'PENDING'`,
		offer.ID,
		f.AuctionID,
		f.DriverID,
		f.AssignmentID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("verify persisted counter-offer: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one persisted counter-offer, got %d", count)
	}
}

func TestFlexCounterOfferHTTPIntegrationRejectsDuplicate(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)
	router := newFlexCounterOfferIntegrationRouter(f)

	first := executeFlexCounterOfferRequest(
		router,
		f.AuctionID,
		`{"proposed_amount_cents":2500}`,
	)

	if first.Code != http.StatusCreated {
		t.Fatalf(
			"first submission expected HTTP 201, got %d: %s",
			first.Code,
			first.Body.String(),
		)
	}

	second := executeFlexCounterOfferRequest(
		router,
		f.AuctionID,
		`{"proposed_amount_cents":2501}`,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		second,
		http.StatusConflict,
	)

	var count int
	var amount string

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT COUNT(*), MIN(offered_amount::text)
         FROM flex_offers
         WHERE auction_id = $1 AND driver_id = $2`,
		f.AuctionID,
		f.DriverID,
	).Scan(&count, &amount)

	if err != nil {
		t.Fatalf("verify duplicate protection: %v", err)
	}

	if count != 1 || amount != "25.00" {
		t.Fatalf(
			"duplicate changed persisted offer: count=%d amount=%s",
			count,
			amount,
		)
	}
}

func TestFlexCounterOfferHTTPIntegrationRejectsUnknownAuction(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)
	router := newFlexCounterOfferIntegrationRouter(f)

	recorder := executeFlexCounterOfferRequest(
		router,
		uuid.NewString(),
		`{"proposed_amount_cents":2500}`,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusNotFound,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexCounterOfferHTTPIntegrationRejectsExpiredAuction(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)

	_, err := f.DB.Exec(
		context.Background(),
		`UPDATE flex_auctions
         SET opened_at = $2,
             expires_at = $3
         WHERE id = $1`,
		f.AuctionID,
		time.Now().UTC().Add(-2*time.Minute),
		time.Now().UTC().Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("expire FLEX auction: %v", err)
	}

	router := newFlexCounterOfferIntegrationRouter(f)

	recorder := executeFlexCounterOfferRequest(
		router,
		f.AuctionID,
		`{"proposed_amount_cents":2500}`,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusConflict,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexCounterOfferHTTPIntegrationRejectsRevokedDocument(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)
	ctx := context.Background()

	var permanentDriverID string

	err := f.DB.QueryRow(
		ctx,
		`SELECT id FROM drivers WHERE user_id = $1`,
		f.DriverID,
	).Scan(&permanentDriverID)
	if err != nil {
		t.Fatalf("resolve permanent driver: %v", err)
	}

	tag, err := f.DB.Exec(
		ctx,
		`UPDATE driver_documents
         SET status = 'REVOKED',
             revoked_at = NOW(),
             revoked_by_user_id = verified_by_user_id,
             revocation_reason = 'CONNECT FLEX counter-offer HTTP integration test',
             updated_at = NOW()
         WHERE driver_id = $1
           AND document_type = $2
           AND status = 'VERIFIED'`,
		permanentDriverID,
		models.DriverDocumentTypeTaxiDriverLicense,
	)
	if err != nil {
		t.Fatalf("revoke driver document: %v", err)
	}

	if tag.RowsAffected() != 1 {
		t.Fatalf(
			"expected one revoked taxi driver licence, got %d",
			tag.RowsAffected(),
		)
	}

	router := newFlexCounterOfferIntegrationRouter(f)

	recorder := executeFlexCounterOfferRequest(
		router,
		f.AuctionID,
		`{"proposed_amount_cents":2500}`,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusConflict,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}
