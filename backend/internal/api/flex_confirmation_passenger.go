package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	flexoffer "github.com/JCKFinland/connect/backend/internal/services/flex_offer"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PassengerFlexRideConfirmer is the existing transaction-safe
// FLEX ride confirmation contract.
type PassengerFlexRideConfirmer interface {
	ConfirmAuthorizedRide(
		ctx context.Context,
		rideRequestID string,
		auctionID string,
		passengerUserID string,
	) (*models.Trip, error)
}

// PassengerFlexConfirmationHandler exposes confirmation only after
// the underlying service verifies the authorized payment.
type PassengerFlexConfirmationHandler struct {
	confirmer PassengerFlexRideConfirmer
}

func NewPassengerFlexConfirmationHandler(
	confirmer PassengerFlexRideConfirmer,
) *PassengerFlexConfirmationHandler {
	return &PassengerFlexConfirmationHandler{
		confirmer: confirmer,
	}
}

type passengerFlexConfirmedTripResponse struct {
	ID            string `json:"id"`
	RideRequestID string `json:"ride_request_id"`
	Status        string `json:"status"`
}

// Confirm does not call Stripe or independently create a trip.
// All financial, ownership, eligibility and concurrency checks
// remain inside ConfirmAuthorizedRide.
func (h *PassengerFlexConfirmationHandler) Confirm(c *gin.Context) {
	if h == nil || h.confirmer == nil {
		response.Error(
			c,
			http.StatusServiceUnavailable,
			"FLEX confirmation service unavailable",
			nil,
		)
		return
	}

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

	trip, err := h.confirmer.ConfirmAuthorizedRide(
		c.Request.Context(),
		rideRequestID,
		auctionID,
		user.ID,
	)

	if err != nil {
		switch {
		case errors.Is(err, flexoffer.ErrFlexConfirmationAccessDenied):
			response.Forbidden(c, "FLEX confirmation access denied")

		case errors.Is(err, flexoffer.ErrFlexConfirmationNotEligible):
			response.Conflict(
				c,
				"FLEX ride is not eligible for confirmation",
			)

		default:
			response.InternalServerError(c)
		}
		return
	}

	if trip == nil || trip.ID == "" ||
		trip.RideRequestID != rideRequestID {
		response.InternalServerError(c)
		return
	}

	response.OK(
		c,
		"FLEX ride confirmed",
		passengerFlexConfirmedTripResponse{
			ID:            trip.ID,
			RideRequestID: trip.RideRequestID,
			Status:        trip.Status,
		},
	)
}
