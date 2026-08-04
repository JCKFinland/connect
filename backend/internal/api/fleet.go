package api

import (
	"net/http"

	// Uses Gin to manage HTTP context, URL routing parameters, and JSON mapping.
	"github.com/gin-gonic/gin"

	// References the business logic package tailored explicitly to taxi fleet metadata.
	"github.com/JCKFinland/connect/backend/internal/services/fleet"

	// Leverages a shared response envelope utility format.
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// CompanyHandler bundles all available HTTP controllers managing company/fleet schemas.
type FleetHandler struct {
	// Points to the structural business engine that communicates with data repositories.
	service *fleet.Service
}

// NewCompanyHandler initializes the struct dependency during application bootstrap in main.go.
func NewFleetHandler(
	service *fleet.Service,
) *FleetHandler {

	return &FleetHandler{
		service: service,
	}
}

// Create handles requests to onboard a brand new taxi company into the ecosystem.
func (h *FleetHandler) Create(
	c *gin.Context,
) {

	var req fleet.CreateFleetRequest

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
		req,
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

	// Extracts the unique ID variable dynamically from the request path (e.g., /companies/:id).
	id := c.Param("id")

	// Queries the service layer to locate the company record.
	fleet, err := h.service.GetByID(
		c.Request.Context(),
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
		"Fleet retrieved successfully",
		fleet,
	)
}

// List pulls every registered company out of the database for dashboards or selectors.
func (h *FleetHandler) List(
	c *gin.Context,
) {

	// Commands the service layer to fetch all available records.
	fleets, err := h.service.List(
		c.Request.Context(),
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
		"Fleets retrieved successfully",
		fleets,
	)
}

// Update changes attributes (e.g., name, phone, status) of an existing company.
func (h *FleetHandler) Update(
	c *gin.Context,
) {

	// Extracts target entity key from URL route parameter.
	id := c.Param("id")

	var req fleet.UpdateFleetRequest

	// Extracts partial structural changes from incoming request payload body.
	if err := c.ShouldBindJSON(&req); err != nil {

		response.Error(
			c,
			http.StatusBadRequest,
			"Invalid request body",
			nil,
		)

		return
	}

	// Injects structural mutations straight to business database logic.
	err := h.service.Update(
		c.Request.Context(),
		id,
		req,
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

	// Acknowledges success with HTTP 200 Status OK.
	response.Success(
		c,
		http.StatusOK,
		"Fleet updated successfully",
		nil,
	)
}

// Delete strips an operating taxi company or fleet permanently out of the database.
func (h *FleetHandler) Delete(
	c *gin.Context,
) {

	// Isolates structural ID parameter string out of path variables.
	id := c.Param("id")

	// Dispatches the deletion intent to the service layer.
	err := h.service.Delete(
		c.Request.Context(),
		id,
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

	// Formats an HTTP 200 Status OK response confirming the company is removed.
	response.Success(
		c,
		http.StatusOK,
		"Fleet deleted successfully",
		nil,
	)
}
