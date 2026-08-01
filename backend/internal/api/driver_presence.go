package api

import (

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type DriverPresenceHandler struct {
	service *presence.Service
}

func NewDriverPresenceHandler(
	service *presence.Service,
) *DriverPresenceHandler {

	return &DriverPresenceHandler{
		service: service,
	}
}

func (h *DriverPresenceHandler) GoOnline(
	c *gin.Context,
) {

	var req presence.GoOnlineRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	if err := h.service.GoOnline(
		c.Request.Context(),
		req,
	); err != nil {

		response.BadRequest(
			c,
			err.Error(),
		)

		return
	}

	response.OK(
		c,
		"Driver is now online",
		nil,
	)
}

func (h *DriverPresenceHandler) GoOffline(
	c *gin.Context,
) {

	var req presence.GoOfflineRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	if err := h.service.GoOffline(
		c.Request.Context(),
		req,
	); err != nil {

		response.BadRequest(
			c,
			err.Error(),
		)

		return
	}

	response.OK(
		c,
		"Driver is offline",
		nil,
	)
}

func (h *DriverPresenceHandler) Heartbeat(
	c *gin.Context,
) {

	var req presence.HeartbeatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	if err := h.service.Heartbeat(
		c.Request.Context(),
		req,
	); err != nil {

		response.BadRequest(
			c,
			err.Error(),
		)

		return
	}

	response.OK(
		c,
		"Heartbeat received",
		nil,
	)
}

func (h *DriverPresenceHandler) UpdateAvailability(
	c *gin.Context,
) {

	var req presence.UpdateAvailabilityRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	if err := h.service.UpdateAvailability(
		c.Request.Context(),
		req,
	); err != nil {

		response.BadRequest(
			c,
			err.Error(),
		)

		return
	}

	response.OK(
		c,
		"Availability updated",
		nil,
	)
}