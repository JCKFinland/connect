package api

import (
	"errors"
	"net/http"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/flex_pricing"
	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type initiateFlexAuctionRequest struct {
	PassengerOfferCents int64 `json:"passenger_offer_cents" binding:"required,gt=0"`
}

// InitiateFlexAuction creates a customer-authorized FLEX auction for an
// eligible ride request using the server-calculated fare estimate.
func (h *RideRequestHandler) InitiateFlexAuction(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	rideID := c.Param("id")
	if _, err := uuid.Parse(rideID); err != nil {
		response.BadRequest(c, "Invalid ride request ID")
		return
	}

	var req initiateFlexAuctionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid FLEX auction request")
		return
	}

	result, err := h.service.InitiateFlexAuctionAuthorized(
		c.Request.Context(),
		rideID,
		user.ID,
		req.PassengerOfferCents,
	)

	if err != nil {
		switch {
		case errors.Is(err, ride_request.ErrFlexAuctionAccessDenied):
			response.Forbidden(
				c,
				"You are not authorized to initiate a FLEX auction for this ride request",
			)

		case errors.Is(err, ride_request.ErrFlexAuctionNotEligible),
			errors.Is(err, ride_request.ErrFlexAuctionExists):
			response.Conflict(
				c,
				"Ride request is not eligible for FLEX auction initiation",
			)

		case errors.Is(err, flex_pricing.ErrInvalidOffer):
			response.BadRequest(c, "Invalid passenger offer")

		case errors.Is(err, flex_pricing.ErrOfferOutOfRange):
			response.UnprocessableEntity(
				c,
				"Passenger offer is outside permitted pricing bounds",
			)

		case errors.Is(err, flex_pricing.ErrInvalidBaseline):
			response.UnprocessableEntity(
				c,
				"Valid FLEX pricing is unavailable",
			)

		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, pgx.ErrNoRows):
			response.NotFound(c, "Ride request not found")

		default:
			response.InternalServerError(c)
		}
		return
	}

	response.Created(c, "FLEX auction initiated", gin.H{
		"auction":             result.Auction,
		"baseline_cents":      result.PricingBounds.BaselineCents,
		"minimum_offer_cents": result.PricingBounds.MinimumCents,
		"maximum_offer_cents": result.PricingBounds.MaximumCents,
		"pricing_profile_id":  result.PricingProfile,
		"pricing_version":     result.PricingVersion,
	})
}

// Compile-time reference for the intended successful HTTP status.
var _ = http.StatusCreated
