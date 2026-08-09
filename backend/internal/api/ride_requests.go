package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/services/ride_request"
)

type RideRequestHandler struct {
	service *ride_request.Service
}

func NewRideRequestHandler(
	service *ride_request.Service,
) *RideRequestHandler {
	return &RideRequestHandler{
		service: service,
	}
}

// Create creates a new ride request.
func (h *RideRequestHandler) Create(c *gin.Context) {
	var req ride_request.CreateRideRequestRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	request, err := h.service.Create(
		c.Request.Context(),
		req,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    request,
	})
}

// GetByID retrieves a ride request.
func (h *RideRequestHandler) GetByID(c *gin.Context) {
	id := c.Param("id")

	request, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    request,
	})
}

// List retrieves ride requests.
func (h *RideRequestHandler) List(c *gin.Context) {
	customerID := c.Query("customer_id")
	status := c.Query("status")

	limit := 50
	offset := 0

	if value := c.Query("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}

	if value := c.Query("offset"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			offset = parsed
		}
	}

	requests, err := h.service.List(
		c.Request.Context(),
		customerID,
		status,
		limit,
		offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    requests,
	})
}

// Update modifies an existing ride request.
func (h *RideRequestHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req ride_request.UpdateRideRequestRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	request, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	request.PickupAddress = req.PickupAddress
	request.PickupLatitude = req.PickupLatitude
	request.PickupLongitude = req.PickupLongitude

	request.DestinationAddress = req.DestinationAddress
	request.DestinationLatitude = req.DestinationLatitude
	request.DestinationLongitude = req.DestinationLongitude

	request.RequestedVehicleType = req.RequestedVehicleType
	request.PassengerCount = req.PassengerCount
	request.Notes = req.Notes
	request.ExpiresAt = req.ExpiresAt

	if err := h.service.Update(
		c.Request.Context(),
		request,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	updated, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updated,
	})
}

// Delete deletes a ride request.
func (h *RideRequestHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.service.Delete(
		c.Request.Context(),
		id,
	); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "ride request deleted successfully",
	})
}

// UpdateStatus changes the lifecycle status of a ride request.
func (h *RideRequestHandler) UpdateStatus(c *gin.Context) {
	id := c.Param("id")

	var req ride_request.UpdateRideRequestStatusRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.UpdateStatus(
		c.Request.Context(),
		id,
		req.Status,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	request, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    request,
	})
}
