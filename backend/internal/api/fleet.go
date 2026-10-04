package api

import (
	"errors"
	"net/http"

	// Uses Gin to manage HTTP context, URL routing parameters, and JSON mapping.
	"github.com/gin-gonic/gin"

	// References the business logic package tailored explicitly to taxi fleet metadata.
	fleetservice "github.com/JCKFinland/connect/backend/internal/services/fleet"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"

	// Leverages a shared response envelope utility format.
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// CompanyHandler bundles all available HTTP controllers managing company/fleet schemas.
type FleetHandler struct {
	// Points to the structural business engine that communicates with data repositories.
	service *fleetservice.Service
}

// NewCompanyHandler initializes the struct dependency during application bootstrap in main.go.
func NewFleetHandler(
	service *fleetservice.Service,
) *FleetHandler {

	return &FleetHandler{
		service: service,
	}
}

// Create handles requests to onboard a brand new taxi company into the ecosystem.
func (h *FleetHandler) Create(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Error(
			c,
			http.StatusUnauthorized,
			"Authentication required",
			nil,
		)
		return
	}

	var req fleetservice.CreateFleetRequest

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
	createdFleet, err := h.service.Create(
		c.Request.Context(),
		user.ID,
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, fleetservice.ErrInvalidBranch):
			response.Error(c, http.StatusNotFound, err.Error(), nil)
		case errors.Is(err, fleetservice.ErrFleetCreationAccessDenied):
			response.Error(c, http.StatusForbidden, err.Error(), nil)
		default:
			response.Error(
				c,
				http.StatusInternalServerError,
				"Failed to create fleet",
				nil,
			)
		}
		return
	}

	// Sends a clear HTTP 201 Status Created back to the client along with the new payload.
	response.Success(
		c,
		http.StatusCreated,
		"Fleet created successfully",
		createdFleet,
	)
}

// GetByID looks up an individual company profile using an identification string.
func (h *FleetHandler) GetByID(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Error(
			c,
			http.StatusUnauthorized,
			"Authentication required",
			nil,
		)
		return
	}

	id := c.Param("id")

	fleet, err := h.service.GetByID(
		c.Request.Context(),
		user.ID,
		id,
	)
	if err != nil {
		if errors.Is(err, fleetservice.ErrFleetNotFound) {
			response.Error(
				c,
				http.StatusNotFound,
				err.Error(),
				nil,
			)
			return
		}

		response.Error(
			c,
			http.StatusInternalServerError,
			"Failed to retrieve fleet",
			nil,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Fleet retrieved successfully",
		fleet,
	)
}

// List pulls every registered company out of the database for dashboards or selectors.
func (h *FleetHandler) List(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Error(
			c,
			http.StatusUnauthorized,
			"Authentication required",
			nil,
		)
		return
	}

	fleets, err := h.service.List(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		response.Error(
			c,
			http.StatusInternalServerError,
			"Failed to retrieve fleets",
			nil,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Fleets retrieved successfully",
		fleets,
	)
}

func (h *FleetHandler) ListForDriver(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Error(
			c,
			http.StatusUnauthorized,
			"Authentication required",
			nil,
		)
		return
	}

	fleets, err := h.service.ListForDriver(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		if errors.Is(err, fleetservice.ErrDriverNotEligible) {
			response.Error(
				c,
				http.StatusForbidden,
				err.Error(),
				nil,
			)
			return
		}

		response.Error(
			c,
			http.StatusInternalServerError,
			"Failed to retrieve driver fleets",
			nil,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Driver fleets retrieved successfully",
		fleets,
	)
}

// Update changes attributes (e.g., name, phone, status) of an existing company.
func (h *FleetHandler) Update(
	c *gin.Context,
) {
	user, exists := middleware.CurrentUser(c)
	if !exists || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	id := c.Param("id")

	var req fleetservice.UpdateFleetRequest

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
		if errors.Is(err, fleetservice.ErrFleetNotFound) {
			response.Error(
				c,
				http.StatusNotFound,
				"Fleet not found",
				nil,
			)
			return
		}

		response.Error(
			c,
			http.StatusInternalServerError,
			"Failed to update fleet",
			nil,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Fleet updated successfully",
		nil,
	)
}

// Delete archives a fleet after enforcing tenant and lifecycle authority.
func (h *FleetHandler) Delete(
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
				"Fleet not found",
				nil,
			)

		case errors.Is(
			err,
			fleetservice.ErrFleetHasVehicles,
		):
			response.Error(
				c,
				http.StatusConflict,
				err.Error(),
				nil,
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Fleet archived successfully",
		nil,
	)
}
