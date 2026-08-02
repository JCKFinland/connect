package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/services/company"

	"github.com/JCKFinland/connect/backend/pkg/response"
)

type CompanyHandler struct {
	service *company.Service
}

func NewCompanyHandler(
	service *company.Service,
) *CompanyHandler {

	return &CompanyHandler{
		service: service,
	}
}


func (h *CompanyHandler) Create(
	c *gin.Context,
) {

	var req company.CreateCompanyRequest

	if err := c.ShouldBindJSON(&req); err != nil {

		response.Error(
			c,
			http.StatusBadRequest,
			"Invalid request body",
			nil,
		)

		return
	}

	createdCompany, err := h.service.Create(
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

	response.Success(
		c,
		http.StatusCreated,
		"Company created successfully",
		createdCompany,
	)
}

func (h *CompanyHandler) GetByID(
	c *gin.Context,
) {

	id := c.Param("id")

	company, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {

		response.Error(
			c,
			http.StatusNotFound,
			err.Error(),
			nil,
		)

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Company retrieved successfully",
		company,
	)
}

func (h *CompanyHandler) List(
	c *gin.Context,
) {

	companies, err := h.service.List(
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

	response.Success(
		c,
		http.StatusOK,
		"Companies retrieved successfully",
		companies,
	)
}

func (h *CompanyHandler) Update(
	c *gin.Context,
) {

	id := c.Param("id")

	var req company.UpdateCompanyRequest

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

	response.Success(
		c,
		http.StatusOK,
		"Company updated successfully",
		nil,
	)
}

func (h *CompanyHandler) Delete(
	c *gin.Context,
) {

	id := c.Param("id")

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

	response.Success(
		c,
		http.StatusOK,
		"Company deleted successfully",
		nil,
	)
}