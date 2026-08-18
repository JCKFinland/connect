package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
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
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authenticated user not found",
		})
		return
	}

	var req ride_request.CreateRideRequestRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	request, err := h.service.CreateAuthorized(
		c.Request.Context(),
		req,
		user.ID,
	)
	if err != nil {
		if errors.Is(err, ride_request.ErrRideRequestAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to create ride requests",
			})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{
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

	if id == "" {
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

	request, err := h.service.GetByIDAuthorized(
		c.Request.Context(),
		id,
		user.ID,
	)
	if err != nil {
		if errors.Is(err, ride_request.ErrRideRequestAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to view this ride request",
			})
			return
		}

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
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authenticated user not found",
		})
		return
	}

	customerID := c.Query("customer_id")
	status := c.Query("status")

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

	requests, err := h.service.ListAuthorized(
		c.Request.Context(),
		user.ID,
		customerID,
		status,
		limit,
		offset,
	)
	if err != nil {
		if errors.Is(err, ride_request.ErrRideRequestAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to list ride requests",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    requests,
		"meta": gin.H{
			"limit":  limit,
			"offset": offset,
			"count":  len(requests),
		},
	})
}

// Update modifies an existing ride request.
func (h *RideRequestHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req ride_request.UpdateRideRequestRequest

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authenticated user not found",
		})
		return
	}

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

	if err := h.service.UpdateAuthorized(
		c.Request.Context(),
		request,
		user.ID,
	); err != nil {

		if errors.Is(err, ride_request.ErrRideRequestAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to update this ride request",
			})
			return
		}

		if errors.Is(err, ride_request.ErrRideRequestNotEditable) {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "Ride request can no longer be edited",
			})
			return
		}

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

// UpdateStatus changes the lifecycle status of a ride request.
func (h *RideRequestHandler) UpdateStatus(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
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

	var req ride_request.UpdateRideRequestStatusRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.UpdateStatusAuthorized(
		c.Request.Context(),
		id,
		req.Status,
		user.ID,
	); err != nil {

		if errors.Is(err, ride_request.ErrRideRequestStatusAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You are not authorized to change this ride request's status",
			})
			return
		}

		if errors.Is(err, ride_request.ErrInvalidRideRequestStatusTransition) {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

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
