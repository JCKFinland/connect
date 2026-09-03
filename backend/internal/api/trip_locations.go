package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type recordTripLocationRequest struct {
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	Altitude       *float64   `json:"altitude,omitempty"`
	SpeedKMH       *float64   `json:"speed_kmh,omitempty"`
	Heading        *int       `json:"heading,omitempty"`
	AccuracyMeters *float64   `json:"accuracy_meters"`
	RecordedAt     *time.Time `json:"recorded_at"`
}

// RecordTripLocation handles POST /api/v1/trips/:id/locations.
func (h *TripHandler) RecordTripLocation(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
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

	var req recordTripLocationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	// Latitude and longitude are pointers here deliberately.
	//
	// A value of 0 is a valid coordinate, so the HTTP layer must
	// distinguish a missing JSON field from an explicitly supplied
	// zero value.
	if req.Latitude == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "latitude is required",
		})
		return
	}

	if req.Longitude == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "longitude is required",
		})
		return
	}

	if req.AccuracyMeters == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "accuracy_meters is required",
		})
		return
	}

	if req.RecordedAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "recorded_at is required",
		})
		return
	}

	location, err := h.service.RecordTripLocation(
		c.Request.Context(),
		id,
		user.ID,
		trip.RecordLocationRequest{
			Latitude:       *req.Latitude,
			Longitude:      *req.Longitude,
			Altitude:       req.Altitude,
			SpeedKMH:       req.SpeedKMH,
			Heading:        req.Heading,
			AccuracyMeters: req.AccuracyMeters,
			RecordedAt:     *req.RecordedAt,
		},
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			trip.ErrTripLocationAccessDenied,
		):
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to record location for this trip",
			})
			return

		case errors.Is(
			err,
			trip.ErrTripNotInProgress,
		):
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "Trip is not in progress",
			})
			return

		case errors.Is(
			err,
			trip.ErrInvalidTripLocation,
		):
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return

		case errors.Is(
			err,
			trip.ErrTripLocationTimestamp,
		):
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return

		case errors.Is(
			err,
			pgx.ErrNoRows,
		):
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to record trip location",
				"error":   err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Trip location recorded successfully",
		"data":    location,
	})
}
