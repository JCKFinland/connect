package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newFlexDriverOfferIntegrationRouter(
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
		flexDriverOfferTestRoute,
		handler.SubmitAcceptFare,
	)

	return router
}

func assertFlexHTTPNoPersistedOffers(
	t *testing.T,
	f *testutil.FlexAcceptFixture,
) {
	t.Helper()

	var count int

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT COUNT(*) FROM flex_offers WHERE auction_id = $1`,
		f.AuctionID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("count FLEX offers: %v", err)
	}

	if count != 0 {
		t.Fatalf("rejected request persisted %d FLEX offers", count)
	}
}

func TestFlexDriverOfferHTTPIntegrationRejectsUnknownAuction(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)
	router := newFlexDriverOfferIntegrationRouter(f)

	recorder := executeFlexDriverOfferRequest(
		router,
		uuid.NewString(),
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusNotFound,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexDriverOfferHTTPIntegrationRejectsExpiredAuction(
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

	router := newFlexDriverOfferIntegrationRouter(f)

	recorder := executeFlexDriverOfferRequest(
		router,
		f.AuctionID,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusConflict,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexDriverOfferHTTPIntegrationRejectsRevokedDocument(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)

	var permanentDriverID string

	err := f.DB.QueryRow(
		context.Background(),
		`SELECT id FROM drivers WHERE user_id = $1`,
		f.DriverID,
	).Scan(&permanentDriverID)
	if err != nil {
		t.Fatalf("resolve permanent driver: %v", err)
	}

	tag, err := f.DB.Exec(
		context.Background(),
		`UPDATE driver_documents
         SET status = 'REVOKED',
             revoked_at = NOW(),
             revoked_by_user_id = verified_by_user_id,
             revocation_reason = 'CONNECT FLEX HTTP integration test',
             updated_at = NOW()
         WHERE driver_id = $1
           AND document_type = $2
           AND status = 'VERIFIED'`,
		permanentDriverID,
		models.DriverDocumentTypeTaxiDriverLicense,
	)
	if err != nil {
		t.Fatalf("revoke driver documents: %v", err)
	}

	if tag.RowsAffected() != 1 {
		t.Fatalf("expected one revoked taxi driver licence, got %d", tag.RowsAffected())
	}

	router := newFlexDriverOfferIntegrationRouter(f)

	recorder := executeFlexDriverOfferRequest(
		router,
		f.AuctionID,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		recorder,
		http.StatusConflict,
	)

	assertFlexHTTPNoPersistedOffers(t, f)
}

func TestFlexDriverOfferHTTPIntegrationSucceeds(t *testing.T) {
	f := testutil.SetupFlexAcceptFixture(t)

	router := newFlexDriverOfferIntegrationRouter(f)

	recorder := executeFlexDriverOfferRequest(
		router,
		f.AuctionID,
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
		Message string           `json:"message"`
		Data    models.FlexOffer `json:"data"`
	}

	if err := json.Unmarshal(
		recorder.Body.Bytes(),
		&result,
	); err != nil {
		t.Fatalf("decode FLEX offer response: %v", err)
	}

	if !result.Success {
		t.Fatal("HTTP 201 response marked unsuccessful")
	}

	offer := result.Data

	if offer.ID == "" ||
		offer.AuctionID != f.AuctionID ||
		offer.DriverID != f.DriverID ||
		offer.AssignmentID != f.AssignmentID ||
		offer.OfferedAmountCents != 2328 ||
		offer.Status != models.FlexOfferStatusPending {
		t.Fatalf("unexpected FLEX offer response: %+v", offer)
	}

	var count int

	err := f.DB.QueryRow(
		context.Background(),
		`
		SELECT COUNT(*)
		FROM flex_offers
		WHERE id = $1
		  AND auction_id = $2
		  AND driver_id = $3
		  AND assignment_id = $4
		  AND offered_amount = 23.28
		  AND status = 'PENDING'
		`,
		offer.ID,
		f.AuctionID,
		f.DriverID,
		f.AssignmentID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("verify persisted FLEX offer: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected one persisted FLEX offer, got %d",
			count,
		)
	}
}

func TestFlexDriverOfferHTTPIntegrationRejectsDuplicate(
	t *testing.T,
) {
	f := testutil.SetupFlexAcceptFixture(t)

	router := newFlexDriverOfferIntegrationRouter(f)

	first := executeFlexDriverOfferRequest(
		router,
		f.AuctionID,
	)

	if first.Code != http.StatusCreated {
		t.Fatalf(
			"first submission expected HTTP 201, got %d: %s",
			first.Code,
			first.Body.String(),
		)
	}

	second := executeFlexDriverOfferRequest(
		router,
		f.AuctionID,
	)

	assertFlexDriverOfferHTTPFailure(
		t,
		second,
		http.StatusConflict,
	)

	var count int

	err := f.DB.QueryRow(
		context.Background(),
		`
		SELECT COUNT(*)
		FROM flex_offers
		WHERE auction_id = $1
		  AND driver_id = $2
		`,
		f.AuctionID,
		f.DriverID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("count persisted FLEX offers: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"duplicate HTTP request created %d FLEX offers",
			count,
		)
	}
}
