package api

import (
	// Injects Gin to read client JSON requests and structure consistent endpoint actions.
	"github.com/gin-gonic/gin"

	// Maps directly to the core driver state-machine business engine.
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// DriverPresenceHandler exposes endpoints verifying real-time tracking metrics.
type DriverPresenceHandler struct {
	// Points to the logical domain engine maintaining the current fleet availability status.
	service *presence.Service
}

// NewDriverPresenceHandler acts as the structural constructor function instantiated inside main.go.
func NewDriverPresenceHandler(
	service *presence.Service,
) *DriverPresenceHandler {

	return &DriverPresenceHandler{
		service: service,
	}
}

// GoOnline marks a driver active, letting dispatch algorithms calculate ride allocations for them.
func (h *DriverPresenceHandler) GoOnline(
	c *gin.Context,
) {

	var req presence.GoOnlineRequest

	// Captures initial pairing criteria (e.g., driver ID, coordinates, vehicle specs) from body.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	// Persists changes through to business validation logic layers.
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

	// Confirms the change back to the client app using an HTTP 200 OK wrapper.
	response.OK(
		c,
		"Driver is now online",
		nil,
	)
}

// GoOffline terminates a driver's active availability shift, removing them from matching sweeps.
func (h *DriverPresenceHandler) GoOffline(
	c *gin.Context,
) {

	var req presence.GoOfflineRequest

	// Validates the inbound payload request signature.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	// Updates operational database indices to drop the driver session cleanly.
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

// Heartbeat handles background keep-alive pings sent periodically by active mobile apps.
func (h *DriverPresenceHandler) Heartbeat(
	c *gin.Context,
) {

	var req presence.HeartbeatRequest

	// Unmarshals periodic telemetry inputs (e.g., current location coordinates, battery status).
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	// Extends the driver's session lifetime index so they are not flagged as disconnected.
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

// UpdateAvailability toggles exact functional driver variations like "Break" or "Accepting Card Only".
func (h *DriverPresenceHandler) UpdateAvailability(
	c *gin.Context,
) {

	var req presence.UpdateAvailabilityRequest

	// Decodes operational state mutations.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	// Notifies the presence service layer to update active driver flags.
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
