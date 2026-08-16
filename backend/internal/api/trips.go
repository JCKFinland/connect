package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/trip"
)

type TripHandler struct {
	service trip.Service
}

func NewTripHandler(service trip.Service) *TripHandler {
	return &TripHandler{
		service: service,
	}
}

// CreateTrip handles POST /api/v1/trips.
func (h *TripHandler) CreateTrip(c *gin.Context) {
	var req trip.CreateTripRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	newTrip := &models.Trip{
		RideRequestID: req.RideRequestID,
		CustomerID:    req.CustomerID,
		DriverID:      req.DriverID,
		VehicleID:     req.VehicleID,
		CompanyID:     req.CompanyID,
		BranchID:      req.BranchID,
		FleetID:       req.FleetID,

		ScheduledAt: req.ScheduledAt,

		PickupAddress:   req.PickupAddress,
		PickupLatitude:  req.PickupLatitude,
		PickupLongitude: req.PickupLongitude,

		DropoffAddress:   req.DropoffAddress,
		DropoffLatitude:  req.DropoffLatitude,
		DropoffLongitude: req.DropoffLongitude,

		PassengerNote: req.PassengerNote,

		EstimatedDistanceKM:      req.EstimatedDistanceKM,
		EstimatedDurationMinutes: req.EstimatedDurationMinutes,
		EstimatedDistanceMeters:  req.EstimatedDistanceMeters,
		EstimatedDurationSeconds: req.EstimatedDurationSeconds,
	}

	if err := h.service.Create(
		c.Request.Context(),
		newTrip,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Trip created successfully",
		"data":    newTrip,
	})

}

// GetTrip handles GET /api/v1/trips/:id.
func (h *TripHandler) GetTrip(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Trip ID is required",
		})
		return
	}

	result, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve trip",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// ListTrips handles GET /api/v1/trips.
func (h *TripHandler) ListTrips(c *gin.Context) {
	companyID := c.Query("company_id")
	branchID := c.Query("branch_id")
	status := c.Query("status")
	driverID := c.Query("driver_id")
	customerID := c.Query("customer_id")

	limit := 50
	offset := 0

	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid limit",
			})
			return
		}

		limit = parsed
	}

	if value := c.Query("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid offset",
			})
			return
		}

		offset = parsed
	}

	results, err := h.service.List(
		c.Request.Context(),
		companyID,
		branchID,
		status,
		driverID,
		customerID,
		limit,
		offset,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"meta": gin.H{
			"limit":  limit,
			"offset": offset,
			"count":  len(results),
		},
	})

}

// UpdateTrip handles PUT /api/v1/trips/:id.
func (h *TripHandler) UpdateTrip(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Trip ID is required",
		})
		return
	}

	var req trip.UpdateTripRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	existingTrip, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve trip",
			"error":   err.Error(),
		})
		return
	}

	existingTrip.ScheduledAt = req.ScheduledAt

	existingTrip.PickupAddress = req.PickupAddress
	existingTrip.PickupLatitude = req.PickupLatitude
	existingTrip.PickupLongitude = req.PickupLongitude

	existingTrip.DropoffAddress = req.DropoffAddress
	existingTrip.DropoffLatitude = req.DropoffLatitude
	existingTrip.DropoffLongitude = req.DropoffLongitude

	existingTrip.PassengerNote = req.PassengerNote

	existingTrip.EstimatedDistanceKM = req.EstimatedDistanceKM
	existingTrip.EstimatedDurationMinutes = req.EstimatedDurationMinutes

	existingTrip.ActualDistanceKM = req.ActualDistanceKM
	existingTrip.ActualDurationMinutes = req.ActualDurationMinutes

	existingTrip.EstimatedDistanceMeters = req.EstimatedDistanceMeters
	existingTrip.EstimatedDurationSeconds = req.EstimatedDurationSeconds

	existingTrip.ActualDistanceMeters = req.ActualDistanceMeters
	existingTrip.ActualDurationSeconds = req.ActualDurationSeconds

	if err := h.service.Update(
		c.Request.Context(),
		existingTrip,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Trip updated successfully",
		"data":    existingTrip,
	})

}

// DeleteTrip handles DELETE /api/v1/trips/:id.
func (h *TripHandler) DeleteTrip(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Trip ID is required",
		})
		return
	}

	if err := h.service.Delete(
		c.Request.Context(),
		id,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete trip",
			"error":   err.Error(),
		})
		return
	}
}

// UpdateTripStatus handles PATCH /api/v1/trips/:id/status.
func (h *TripHandler) UpdateTripStatus(c *gin.Context) {
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

	var req trip.UpdateTripStatusRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.UpdateStatus(
		c.Request.Context(),
		id,
		req.Status,
		user.ID,
	); err != nil {

		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Trip status updated successfully",
	})
}

// AssignDriver handles POST /api/v1/trips/:id/assign.
func (h *TripHandler) AssignDriver(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Trip ID is required",
		})
		return
	}

	var req trip.AssignDriverRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.AssignDriver(
		c.Request.Context(),
		id,
		req.DriverID,
		req.VehicleID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Trip not found",
			})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Driver and vehicle assigned successfully",
	})

}
