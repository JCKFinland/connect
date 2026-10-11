package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

// GetTripByRideRequest resolves the trip created when a dispatch offer is accepted.
func (h *TripHandler) GetTripByRideRequest(c *gin.Context) {
	rideRequestID := c.Param("id")
	if rideRequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Ride request ID is required",
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

	result, err := h.service.GetByRideRequestIDAuthorized(
		c.Request.Context(),
		rideRequestID,
		user.ID,
	)
	if err != nil {
		switch {
		case errors.Is(err, ride_request.ErrRideRequestAccessDenied),
			errors.Is(err, trip.ErrTripAccessDenied):
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to view this trip",
			})

		case errors.Is(err, pgx.ErrNoRows),
			errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to retrieve trip",
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
