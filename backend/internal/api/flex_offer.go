package api

import (
	"errors"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FlexOfferHandler exposes authenticated FLEX driver operations.
type FlexOfferHandler struct {
	service *flex_offer.Service
}

func NewFlexOfferHandler(
	db *pgxpool.Pool,
	cfg *config.Config,
) *FlexOfferHandler {
	return &FlexOfferHandler{
		service: flex_offer.NewService(
			flex_offer.Dependencies{
				DB:     db,
				Config: cfg,
			},
		),
	}
}

// SubmitAcceptFare submits the authenticated driver's acceptance
// of the passenger's immutable FLEX auction fare.
func (h *FlexOfferHandler) SubmitAcceptFare(c *gin.Context) {
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

	offer, err := h.service.SubmitAcceptFare(
		c.Request.Context(),
		auctionID,
		user.ID,
	)

	if err != nil {
		switch {
		case errors.Is(err, flex_offer.ErrAccessDenied):
			response.Forbidden(
				c,
				"You are not authorized to submit FLEX offers",
			)

		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, pgx.ErrNoRows):
			response.NotFound(c, "FLEX auction not found")

		case errors.Is(err, flex_offer.ErrAuctionNotEligible):
			response.Conflict(
				c,
				"FLEX auction is no longer eligible for offers",
			)

		case errors.Is(err, flex_offer.ErrDriverNotEligible):
			response.Conflict(
				c,
				"Driver is not eligible to submit this FLEX offer",
			)

		case errors.Is(err, flex_offer.ErrOfferAlreadyExists):
			response.Conflict(
				c,
				"Driver has already submitted an offer for this auction",
			)

		default:
			response.InternalServerError(c)
		}
		return
	}

	response.Created(c, "FLEX driver offer submitted", offer)
}
