package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/realtime"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

const tripStreamHeartbeatInterval = 15 * time.Second

type TripStreamHandler struct {
	trips    trip.Service
	broker   *realtime.Broker
	shutdown <-chan struct{}
}

func NewTripStreamHandler(
	trips trip.Service,
	broker *realtime.Broker,
	shutdown ...<-chan struct{},
) *TripStreamHandler {
	var shutdownCh <-chan struct{}

	if len(shutdown) > 0 {
		shutdownCh = shutdown[0]
	}

	return &TripStreamHandler{
		trips:    trips,
		broker:   broker,
		shutdown: shutdownCh,
	}
}

// Stream handles GET /api/v1/trips/:id/stream.
//
// The persisted trip and location tables remain the durable source
// of truth. This endpoint only provides live notifications.
func (h *TripStreamHandler) Stream(c *gin.Context) {
	tripID := c.Param("id")

	if tripID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Trip ID is required",
		})
		return
	}

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authenticated user not found",
		})
		return
	}

	// Reuse CONNECT's canonical trip read authorization.
	if _, err := h.trips.GetByIDAuthorized(
		c.Request.Context(),
		tripID,
		user.ID,
	); err != nil {
		if errors.Is(
			err,
			trip.ErrTripAccessDenied,
		) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to stream this trip",
			})
			return
		}

		if errors.Is(
			err,
			repository.ErrNotFound,
		) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to authorize trip stream",
		})
		return
	}

	if h.broker == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Realtime broker is not configured",
		})
		return
	}

	// CONNECT's normal HTTP requests keep the server-wide
	// WriteTimeout. Only this long-lived SSE response clears
	// its write deadline.
	controller := http.NewResponseController(c.Writer)

	if err := controller.SetWriteDeadline(
		time.Time{},
	); err != nil &&
		!errors.Is(err, http.ErrNotSupported) {

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to configure realtime stream",
		})
		return
	}

	topic := "trip:" + tripID

	events, unsubscribe :=
		h.broker.Subscribe(topic)

	defer unsubscribe()

	c.Header(
		"Content-Type",
		"text/event-stream",
	)
	c.Header(
		"Cache-Control",
		"no-cache",
	)
	c.Header(
		"Connection",
		"keep-alive",
	)
	c.Header(
		"X-Accel-Buffering",
		"no",
	)

	heartbeat := time.NewTicker(
		tripStreamHeartbeatInterval,
	)
	defer heartbeat.Stop()

	c.SSEvent(
		"connected",
		gin.H{
			"trip_id": tripID,
		},
	)
	c.Writer.Flush()

	for {
		select {
		case <-h.shutdown:
			return

		case <-c.Request.Context().Done():
			return

		case event := <-events:
			c.SSEvent(
				event.Type,
				event.Data,
			)
			c.Writer.Flush()

		case <-heartbeat.C:
			c.SSEvent(
				"heartbeat",
				gin.H{
					"trip_id": tripID,
				},
			)
			c.Writer.Flush()
		}
	}
}
