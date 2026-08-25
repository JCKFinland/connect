package api

import (
	// Injects Gin to read client JSON requests and structure consistent endpoint actions.
	"errors"

	"github.com/gin-gonic/gin"

	// Maps directly to the core driver state-machine business engine.
	"github.com/JCKFinland/connect/backend/internal/middleware"
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

func handlePresenceError(
	c *gin.Context,
	err error,
) {
	switch {
	case errors.Is(
		err,
		presence.ErrInvalidLatitude,
	),
		errors.Is(
			err,
			presence.ErrInvalidLongitude,
		),
		errors.Is(
			err,
			presence.ErrInvalidHeading,
		),
		errors.Is(
			err,
			presence.ErrInvalidSpeed,
		),
		errors.Is(
			err,
			presence.ErrInvalidAccuracy,
		):

		response.BadRequest(
			c,
			err.Error(),
		)

	case errors.Is(
		err,
		presence.ErrDriverNotFound,
	):

		response.NotFound(
			c,
			err.Error(),
		)

	case errors.Is(
		err,
		presence.ErrDriverAssignmentRequired,
	),
		errors.Is(
			err,
			presence.ErrDriverAvailabilityLocked,
		),
		errors.Is(
			err,
			presence.ErrDriverHeartbeatUnavailable,
		):

		response.Conflict(
			c,
			err.Error(),
		)

	default:
		response.InternalServerError(c)
	}
}

// GoOnline marks a driver active, letting dispatch algorithms calculate ride allocations for them.
func (h *DriverPresenceHandler) GoOnline(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req presence.GoOnlineRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// The user identity comes from the authenticated JWT,
	// never from the client request.
	req.UserID = user.ID

	if err := h.service.GoOnline(
		c.Request.Context(),
		req,
	); err != nil {
		handlePresenceError(
			c,
			err,
		)
		return
	}

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

	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Unauthorized(c, "authenticated user not found")
		return
	}

	req.UserID = user.ID

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

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			err.Error(),
		)
		return
	}

	// Authentication determines the driver identity.
	// Never trust the request body for this value.
	req.UserID = user.ID

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

	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Unauthorized(c, "authenticated user not found")
		return
	}

	req.UserID = user.ID

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

// ListAvailable returns drivers who are currently online
// and available within the authenticated driver's company.
func (h *DriverPresenceHandler) ListAvailable(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	drivers, err := h.service.ListAvailableForUser(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		if errors.Is(
			err,
			presence.ErrDriverNotFound,
		) {
			response.NotFound(
				c,
				err.Error(),
			)
			return
		}

		response.InternalServerError(c)
		return
	}

	response.OK(
		c,
		"Available drivers retrieved successfully",
		drivers,
	)
}
