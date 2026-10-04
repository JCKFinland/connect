package api

import (
	"errors"
	"net/http"

	// Uses Gin to manage HTTP context, URL routing parameters, and JSON mapping.
	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"

	// References the business logic package tailored explicitly to taxi fleet metadata.
	"github.com/JCKFinland/connect/backend/internal/services/vehicle"

	// Leverages a shared response envelope utility format.
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// CompanyHandler bundles all available HTTP controllers managing company/fleet schemas.
type VehicleHandler struct {
	// Points to the structural business engine that communicates with data repositories.
	service *vehicle.Service
}

// NewCompanyHandler initializes the struct dependency during application bootstrap in main.go.
func NewVehicleHandler(
	service *vehicle.Service,
) *VehicleHandler {

	return &VehicleHandler{
		service: service,
	}
}

// Create handles requests to onboard a brand new taxi company into the ecosystem.
func (h *VehicleHandler) Create(
	c *gin.Context,
) {

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req vehicle.CreateVehicleRequest

	// Validates and maps incoming JSON fields onto the expected request structure.
	if err := c.ShouldBindJSON(&req); err != nil {

		response.Error(
			c,
			http.StatusBadRequest,
			"Invalid request body",
			nil,
		)

		return
	}

	// Forwards the data payload to the underlying business service layer.
	createdVehicle, err := h.service.Create(
		c.Request.Context(),
		user.ID,
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, vehicle.ErrVehicleCreationAccessDenied):
			response.Forbidden(c, err.Error())

		case errors.Is(err, vehicle.ErrInvalidFleet):
			response.BadRequest(c, err.Error())

		default:
			response.InternalServerError(c)
		}

		return
	}

	// Sends a clear HTTP 201 Status Created back to the client along with the new payload.
	response.Success(
		c,
		http.StatusCreated,
		"Vehicle created successfully",
		createdVehicle,
	)
}

// GetByID looks up an individual company profile using an identification string.
func (h *VehicleHandler) GetByID(
	c *gin.Context,
) {

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	id := c.Param("id")

	vehicle, err := h.service.GetByID(
		c.Request.Context(),
		user.ID,
		id,
	)
	if err != nil {

		// Returns an HTTP 404 Status Not Found if the company ID doesn't match an active record.
		response.Error(
			c,
			http.StatusNotFound,
			err.Error(),
			nil,
		)

		return
	}

	// Returns an HTTP 200 Status OK with the corresponding company metadata object.
	response.Success(
		c,
		http.StatusOK,
		"Vehicle retrieved successfully",
		vehicle,
	)
}

// List pulls every registered company out of the database for dashboards or selectors.
func (h *VehicleHandler) List(
	c *gin.Context,
) {

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	vehicles, err := h.service.List(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {

		response.Error(
			c,
			http.StatusInternalServerError,
			err.Error(),
			nil,
		)

		return
	}

	// Returns an HTTP 200 Status OK containing the array of companies.
	response.Success(
		c,
		http.StatusOK,
		"Vehicles retrieved successfully",
		vehicles,
	)
}

// Update changes descriptive fields of a vehicle within the authenticated
// user's tenant authority.
func (h *VehicleHandler) Update(
	c *gin.Context,
) {
	user, exists := middleware.CurrentUser(c)
	if !exists {
		response.Error(
			c,
			http.StatusUnauthorized,
			"Unauthorized",
			nil,
		)
		return
	}

	id := c.Param("id")

	var req vehicle.UpdateVehicleRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(
			c,
			http.StatusBadRequest,
			"Invalid request body",
			nil,
		)
		return
	}

	err := h.service.Update(
		c.Request.Context(),
		user.ID,
		id,
		req,
	)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			response.Error(
				c,
				http.StatusNotFound,
				"Vehicle not found",
				nil,
			)
			return
		}

		response.Error(
			c,
			http.StatusInternalServerError,
			err.Error(),
			nil,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Vehicle updated successfully",
		nil,
	)
}

// Delete archives a vehicle within the authenticated user's tenant authority.
func (h *VehicleHandler) Delete(
	c *gin.Context,
) {

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	id := c.Param("id")

	err := h.service.Delete(
		c.Request.Context(),
		user.ID,
		id,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			repository.ErrNotFound,
		):
			response.Error(
				c,
				http.StatusNotFound,
				"Vehicle not found",
				nil,
			)

		case errors.Is(
			err,
			vehicle.ErrVehicleHasActiveAssignment,
		):
			response.Error(
				c,
				http.StatusConflict,
				err.Error(),
				nil,
			)

		case errors.Is(
			err,
			vehicle.ErrVehicleCreationAccessDenied,
		):
			response.Forbidden(
				c,
				err.Error(),
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Vehicle archived successfully",
		nil,
	)
}

// Deactivate removes a vehicle from operational eligibility while preserving
// its administrative and historical record.
func (h *VehicleHandler) Deactivate(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	id := c.Param("id")

	err := h.service.Deactivate(
		c.Request.Context(),
		user.ID,
		id,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			repository.ErrNotFound,
		):
			response.Error(
				c,
				http.StatusNotFound,
				"Vehicle not found",
				nil,
			)

		case errors.Is(
			err,
			vehicle.ErrVehicleHasActiveAssignment,
		):
			response.Error(
				c,
				http.StatusConflict,
				err.Error(),
				nil,
			)

		case errors.Is(
			err,
			vehicle.ErrVehicleCreationAccessDenied,
		):
			response.Forbidden(
				c,
				err.Error(),
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Vehicle deactivated successfully",
		nil,
	)
}

// Reactivate restores operational eligibility for a vehicle whose owning fleet
// is itself operationally active.
func (h *VehicleHandler) Reactivate(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	id := c.Param("id")

	err := h.service.Reactivate(
		c.Request.Context(),
		user.ID,
		id,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			repository.ErrNotFound,
		):
			response.Error(
				c,
				http.StatusNotFound,
				"Vehicle not found",
				nil,
			)

		case errors.Is(
			err,
			vehicle.ErrVehicleFleetInactive,
		):
			response.Error(
				c,
				http.StatusConflict,
				err.Error(),
				nil,
			)

		case errors.Is(
			err,
			vehicle.ErrVehicleCreationAccessDenied,
		):
			response.Forbidden(
				c,
				err.Error(),
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Vehicle reactivated successfully",
		nil,
	)
}

// RegisterForDriver registers a vehicle for the authenticated driver.
func (h *VehicleHandler) RegisterForDriver(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req vehicle.RegisterDriverVehicleRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	createdVehicle, err := h.service.Register(
		c.Request.Context(),
		user.ID,
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, vehicle.ErrInvalidVehicle),
			errors.Is(err, vehicle.ErrInvalidFleet):
			response.BadRequest(c, err.Error())

		case errors.Is(err, vehicle.ErrDriverNotEligible):
			response.Forbidden(c, err.Error())

		case errors.Is(err, vehicle.ErrDuplicateRegistrationNumber),
			errors.Is(err, vehicle.ErrDuplicateVIN):
			response.Conflict(c, err.Error())

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Created(
		c,
		"Vehicle registered successfully",
		createdVehicle,
	)
}
