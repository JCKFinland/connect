package api

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type flexCounterOfferRequest struct {
	ProposedAmountCents int64 `json:"proposed_amount_cents"`
}

// SubmitCounterOffer submits a driver's proposed FLEX fare.
// The authenticated user identity comes exclusively from middleware.
func (h *FlexOfferHandler) SubmitCounterOffer(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	auctionID := c.Param("auction_id")
	if _, err := uuid.Parse(auctionID); err != nil {
		response.BadRequest(c, "Invalid FLEX auction ID")
		return
	}

	const maxBodyBytes = 4096

	body, err := io.ReadAll(
		io.LimitReader(c.Request.Body, maxBodyBytes+1),
	)
	if err != nil || len(body) > maxBodyBytes {
		response.BadRequest(c, "Invalid counter-offer request body")
		return
	}

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()

	var request flexCounterOfferRequest
	if err := decoder.Decode(&request); err != nil {
		response.BadRequest(c, "Invalid counter-offer request body")
		return
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid counter-offer request body")
		return
	}

	if request.ProposedAmountCents <= 0 {
		response.BadRequest(c, "Invalid counter-offer amount")
		return
	}

	offer, err := h.service.SubmitCounterOffer(
		c.Request.Context(),
		auctionID,
		user.ID,
		request.ProposedAmountCents,
	)
	if err != nil {
		switch {
		case errors.Is(err, flex_offer.ErrInvalidCounterOfferAmount):
			response.BadRequest(c, "Invalid counter-offer amount")

		case errors.Is(err, flex_offer.ErrAccessDenied):
			response.Forbidden(c, "You are not authorized to submit FLEX offers")

		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, pgx.ErrNoRows):
			response.NotFound(c, "FLEX auction not found")

		case errors.Is(err, flex_offer.ErrAuctionNotEligible):
			response.Conflict(c, "FLEX auction is no longer eligible for offers")

		case errors.Is(err, flex_offer.ErrDriverNotEligible):
			response.Conflict(c, "Driver is not eligible to submit this FLEX offer")

		case errors.Is(err, flex_offer.ErrOfferAlreadyExists):
			response.Conflict(c, "Driver has already submitted an offer for this auction")

		default:
			response.InternalServerError(c)
		}
		return
	}

	response.Created(c, "FLEX driver counter-offer submitted", offer)
}
