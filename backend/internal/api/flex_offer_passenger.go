package api

import (
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// passengerFlexOfferResponse is an explicit public projection.
// Internal operational identifiers must never be serialized here.
type passengerFlexOfferResponse struct {
	ID                 string    `json:"id"`
	OfferedAmountCents int64     `json:"offered_amount_cents"`
	OfferType          string    `json:"offer_type"`
	Status             string    `json:"status"`
	OfferedAt          time.Time `json:"offered_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}

func toPassengerFlexOfferResponse(
	offer *models.FlexOffer,
) passengerFlexOfferResponse {
	return passengerFlexOfferResponse{
		ID:                 offer.ID,
		OfferedAmountCents: offer.OfferedAmountCents,
		OfferType:          offer.OfferType,
		Status:             offer.Status,
		OfferedAt:          offer.OfferedAt,
		ExpiresAt:          offer.ExpiresAt,
	}
}

// ListPassengerOffers exposes only pending, unexpired offers
// for a FLEX auction owned by the authenticated passenger.
//
// This endpoint does not select offers, authorize payments,
// reserve drivers, or create trips.
func (h *FlexOfferHandler) ListPassengerOffers(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	rideRequestID := c.Param("id")
	auctionID := c.Param("auction_id")

	if _, err := uuid.Parse(rideRequestID); err != nil {
		response.BadRequest(c, "Invalid ride request ID")
		return
	}

	if _, err := uuid.Parse(auctionID); err != nil {
		response.BadRequest(c, "Invalid FLEX auction ID")
		return
	}

	offers, err := h.service.ListPassengerOffers(
		c.Request.Context(),
		rideRequestID,
		auctionID,
		user.ID,
	)

	if err != nil {
		switch {
		case errors.Is(err, flex_offer.ErrPassengerOfferAccessDenied):
			response.Forbidden(
				c,
				"You are not authorized to view these FLEX offers",
			)

		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, pgx.ErrNoRows):
			response.NotFound(c, "FLEX auction not found")

		case errors.Is(err, flex_offer.ErrPassengerOfferAuctionNotEligible):
			response.Conflict(
				c,
				"FLEX auction is no longer eligible for offer discovery",
			)

		default:
			response.InternalServerError(c)
		}
		return
	}

	publicOffers := make(
		[]passengerFlexOfferResponse,
		0,
		len(offers),
	)

	for _, offer := range offers {
		publicOffers = append(
			publicOffers,
			toPassengerFlexOfferResponse(offer),
		)
	}

	response.OK(
		c,
		"FLEX passenger offers retrieved",
		publicOffers,
	)
}
