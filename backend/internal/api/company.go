package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	companyservice "github.com/JCKFinland/connect/backend/internal/services/company"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type CompanyHandler struct {
	service *companyservice.Service
}

func NewCompanyHandler(
	service *companyservice.Service,
) *CompanyHandler {
	return &CompanyHandler{
		service: service,
	}
}

func (h *CompanyHandler) Create(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req companyservice.CreateCompanyRequest
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
		user.ID,
		req,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			companyservice.ErrCompanyCreationAccessDenied,
		):
			response.Error(
				c,
				http.StatusForbidden,
				"Company creation access denied",
				nil,
			)

		default:
			response.Error(
				c,
				http.StatusInternalServerError,
				"Failed to create company",
				nil,
			)
		}
		return
	}

	response.Success(
		c,
		http.StatusCreated,
		"Company created successfully",
		createdCompany,
	)
}

func (h *CompanyHandler) GetByID(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	companyObj, err := h.service.GetByID(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, companyservice.ErrCompanyNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Company not found",
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
		"Company retrieved successfully",
		companyObj,
	)
}

func (h *CompanyHandler) List(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	companies, err := h.service.List(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		response.InternalServerError(c)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Companies retrieved successfully",
		companies,
	)
}

func (h *CompanyHandler) Update(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req companyservice.UpdateCompanyRequest
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
		c.Param("id"),
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, companyservice.ErrCompanyNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Company not found",
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
		"Company updated successfully",
		nil,
	)
}

// Delete archives a company. It does not cascade to branches.
func (h *CompanyHandler) Delete(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	err := h.service.Delete(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, companyservice.ErrCompanyNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Company not found",
				nil,
			)

		case errors.Is(err, companyservice.ErrCompanyHasBranches):
			response.Error(
				c,
				http.StatusConflict,
				"Company contains non-archived branches",
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
		"Company archived successfully",
		nil,
	)
}

func (h *CompanyHandler) Deactivate(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	err := h.service.Deactivate(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, companyservice.ErrCompanyNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Company not found",
				nil,
			)

		case errors.Is(
			err,
			companyservice.ErrCompanyHasActiveBranches,
		):
			response.Error(
				c,
				http.StatusConflict,
				"Company cannot be deactivated while it contains active branches",
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
		"Company deactivated successfully",
		nil,
	)
}

func (h *CompanyHandler) Reactivate(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	err := h.service.Reactivate(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, companyservice.ErrCompanyNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Company not found",
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
		"Company reactivated successfully",
		nil,
	)
}
