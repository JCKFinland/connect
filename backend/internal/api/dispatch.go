package api

import (
	"errors"
	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/services/dispatch"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// DispatchHandler exposes ride-dispatch operations.
type DispatchHandler struct {
	service *dispatch.Service
}

// NewDispatchHandler creates a dispatch API handler.
func NewDispatchHandler(
	service *dispatch.Service,
) *DispatchHandler {
	return &DispatchHandler{
		service: service,
	}
}

// DispatchRide selects the nearest eligible driver and creates
// a PENDING dispatch offer.
//
// The ride request becomes MATCHING, but no trip is created and
// the selected driver remains AVAILABLE until the offer is accepted.
func (h *DispatchHandler) DispatchRide(
	c *gin.Context,
) {
	rideRequestID := c.Param("id")

	if rideRequestID == "" {
		response.BadRequest(
			c,
			"ride request ID is required",
		)
		return
	}

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	offer, err := h.service.CreateOffer(
		c.Request.Context(),
		rideRequestID,
		user.ID,
	)
	if err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	response.OK(
		c,
		"Ride offer created successfully",
		offer,
	)
}

// AcceptOffer handles driver acceptance of a dispatch offer.
func (h *DispatchHandler) AcceptOffer(
	c *gin.Context,
) {
	offerID := c.Param("offer_id")

	if offerID == "" {
		response.BadRequest(
			c,
			"dispatch offer ID is required",
		)
		return
	}

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	trip, err := h.service.AcceptOfferAuthorized(
		c.Request.Context(),
		offerID,
		user.ID,
	)
	if err != nil {

		if errors.Is(
			err,
			dispatch.ErrDispatchOfferAccessDenied,
		) {
			response.Forbidden(
				c,
				"You are not authorized to accept this dispatch offer",
			)
			return
		}

		if errors.Is(
			err,
			dispatch.ErrDispatchOfferAlreadyResolved,
		) {
			response.BadRequest(
				c,
				"Dispatch offer is already resolved",
			)
			return
		}

		if errors.Is(
			err,
			dispatch.ErrDispatchOfferExpired,
		) {
			response.BadRequest(
				c,
				"Dispatch offer has expired",
			)
			return
		}

		if errors.Is(
			err,
			dispatch.ErrDispatchOfferDriverUnavailable,
		) {
			response.BadRequest(
				c,
				"Driver is no longer available",
			)
			return
		}

		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	response.OK(
		c,
		"Dispatch offer accepted successfully",
		trip,
	)
}
