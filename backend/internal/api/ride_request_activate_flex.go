package api

import (
	"errors"
	"net/http"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ActivateFlex converts an eligible, customer-owned STANDARD ride request
// to FLEX matching before STANDARD dispatch has started.
func (h *RideRequestHandler) ActivateFlex(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authenticated user not found",
		})
		return
	}

	rideID := c.Param("id")
	if _, err := uuid.Parse(rideID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid ride request ID",
		})
		return
	}

	err := h.service.ActivateFlexAuthorized(
		c.Request.Context(),
		rideID,
		user.ID,
	)

	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "FLEX matching activated",
			"data": gin.H{
				"ride_request_id": rideID,
				"matching_mode":   "FLEX",
			},
		})

	case errors.Is(err, ride_request.ErrFlexActivationAccessDenied):
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "You are not authorized to activate FLEX for this ride request",
		})

	case errors.Is(err, ride_request.ErrFlexActivationNotEligible):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Ride request is not eligible for FLEX activation",
		})

	case errors.Is(err, repository.ErrNotFound),
		errors.Is(err, pgx.ErrNoRows):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Ride request not found",
		})

	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to activate FLEX matching",
		})
	}
}
