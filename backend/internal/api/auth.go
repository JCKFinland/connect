package api

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/repository"
	authservice "github.com/JCKFinland/connect/backend/internal/services/auth"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type AuthHandler struct {
	service *authservice.AuthService
}

func NewAuthHandler(service *authservice.AuthService) *AuthHandler {
	return &AuthHandler{
		service: service,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {

	var req authservice.RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	user, err := h.service.Register(c.Request.Context(), req)
	if err != nil {

		switch {

		case errors.Is(err, repository.ErrEmailAlreadyUsed):
			response.Conflict(c, err.Error())

		default:
			response.InternalServerError(c)
			println("Refresh error:", err.Error())
		}

		return
	}

	response.Created(
		c,
		"User registered successfully",
		user,
	)
}

func (h *AuthHandler) Login(c *gin.Context) {

	var req authservice.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	result, err := h.service.Login(c.Request.Context(), req)
	if err != nil {

		switch {

		case errors.Is(err, repository.ErrInvalidCredentials):
			response.Unauthorized(c, err.Error())

		case errors.Is(err, repository.ErrAccountDisabled):
			response.Forbidden(c, err.Error())

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.OK(
		c,
		"Login successful",
		result,
	)
}

func (h *AuthHandler) Refresh(c *gin.Context) {

	var req authservice.RefreshTokenRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	result, err := h.service.Refresh(c.Request.Context(), req)
	if err != nil {

		switch {

		case errors.Is(err, repository.ErrInvalidRefreshToken):
			response.Unauthorized(c, err.Error())

		case errors.Is(err, repository.ErrAccountDisabled):
			response.Forbidden(c, err.Error())

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.OK(
		c,
		"Token refreshed successfully",
		result,
	)
}

func (h *AuthHandler) Logout(c *gin.Context) {

	var req authservice.RefreshTokenRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.service.Logout(c.Request.Context(), req); err != nil {
		response.InternalServerError(c)
		return
	}

	response.OK(
		c,
		"Logout successful",
		nil,
	)
}
