package api

import (
	"errors"

	// Uses the Gin web framework to process HTTP contexts, headers, and payloads.
	"github.com/gin-gonic/gin"

	// Imports database core error types to translate database outcomes into client codes.
	"github.com/JCKFinland/connect/backend/internal/repository"
	authservice "github.com/JCKFinland/connect/backend/internal/services/auth"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// AuthHandler bundles the endpoints required to securely log users and drivers in or out.
type AuthHandler struct {
	// Points directly to the business logic layer responsible for crypto and data handling.
	service *authservice.AuthService
}

// NewAuthHandler acts as the constructor function invoked inside main.go.
func NewAuthHandler(service *authservice.AuthService) *AuthHandler {
	return &AuthHandler{
		service: service,
	}
}

// Register processes requests from new passengers or drivers trying to sign up.
func (h *AuthHandler) Register(c *gin.Context) {

	var req authservice.RegisterRequest

	// Decodes inbound client JSON fields directly into the structured Go model.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	// Passes execution context and user details down to the logic layer.
	user, err := h.service.Register(c.Request.Context(), req)
	if err != nil {

		// Switches status code safely depending on the core failure type.
		switch {

		// Returns an HTTP 409 Conflict error if the email is already in use.
		case errors.Is(err, repository.ErrEmailAlreadyUsed):
			response.Conflict(c, err.Error())

		// Defaults to an HTTP 500 Internal Server Error for unhandled exceptions.
		default:
			response.InternalServerError(c)
			println("Refresh error:", err.Error())
		}

		return
	}

	// Returns an HTTP 201 Created response along with the newly saved user record.
	response.Created(
		c,
		"User registered successfully",
		user,
	)
}

// Login manages user/driver authentication checks.
func (h *AuthHandler) Login(c *gin.Context) {

	var req authservice.LoginRequest

	// Validates that the submitted login credentials structure is correct JSON.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	// Executes password checking and generates new tokens via the authService.
	result, err := h.service.Login(c.Request.Context(), req)
	if err != nil {

		switch {

		// Returns an HTTP 401 Unauthorized code if the email or password fails checks.
		case errors.Is(err, repository.ErrInvalidCredentials):
			response.Unauthorized(c, err.Error())

		// Returns an HTTP 403 Forbidden code if a driver or user was banned or deactivated.
		case errors.Is(err, repository.ErrAccountDisabled):
			response.Forbidden(c, err.Error())

		default:
			response.InternalServerError(c)
		}

		return
	}

	// Returns an HTTP 200 OK code along with the fresh Access Token and Refresh Token.
	response.OK(
		c,
		"Login successful",
		result,
	)
}

// Refresh handles token rotation when the user's short-lived Access Token expires.
func (h *AuthHandler) Refresh(c *gin.Context) {

	var req authservice.RefreshTokenRequest

	// Captures the current active refresh token from the request body.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	// Validates the old refresh token and generates a brand new token set.
	result, err := h.service.Refresh(c.Request.Context(), req)
	if err != nil {

		switch {

		// Returns an HTTP 401 Unauthorized code if the token is expired or altered.
		case errors.Is(err, repository.ErrInvalidRefreshToken):
			response.Unauthorized(c, err.Error())

		// Returns an HTTP 403 Forbidden code if the user's account state has changed.
		case errors.Is(err, repository.ErrAccountDisabled):
			response.Forbidden(c, err.Error())

		default:
			response.InternalServerError(c)
		}

		return
	}

	// Returns an HTTP 200 OK status containing the new rotated credentials.
	response.OK(
		c,
		"Token refreshed successfully",
		result,
	)
}

// Logout terminates user sessions safely.
func (h *AuthHandler) Logout(c *gin.Context) {

	var req authservice.RefreshTokenRequest

	// Captures the targeted refresh token to terminate it.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	// Drops or invalidates the token record from the persistent database.
	if err := h.service.Logout(c.Request.Context(), req); err != nil {
		response.InternalServerError(c)
		return
	}

	// Returns an HTTP 200 OK status confirming complete session destruction.
	response.OK(
		c,
		"Logout successful",
		nil,
	)
}
