package api

import (
	"github.com/gin-gonic/gin"

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

// DispatchRide matches an available driver to a pending ride request
// and creates the assigned trip atomically.
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

	trip, err := h.service.DispatchRide(
		c.Request.Context(),
		rideRequestID,
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
		"Ride dispatched successfully",
		trip,
	)
}
