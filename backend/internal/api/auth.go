package api

import (
	"errors"
	"net/http"
	"time"

	// Uses the Gin web framework to process HTTP contexts, headers, and payloads.
	"github.com/gin-gonic/gin"

	// Imports database core error types to translate database outcomes into client codes.
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	authservice "github.com/JCKFinland/connect/backend/internal/services/auth"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

const (
	refreshTokenCookieName = "connect_refresh_token"
	refreshTokenCookiePath = "/api/v1/auth"
)

// AuthHandler bundles the endpoints required to securely log users and drivers in or out.
type AuthHandler struct {
	service *authservice.AuthService
	cfg     *config.Config
}

// NewAuthHandler acts as the constructor function invoked inside main.go.
func NewAuthHandler(
	service *authservice.AuthService,
	cfg *config.Config,
) *AuthHandler {
	return &AuthHandler{
		service: service,
		cfg:     cfg,
	}
}

func (h *AuthHandler) refreshCookieSecure() bool {
	return h.cfg != nil &&
		h.cfg.App.Env != "development"
}

func (h *AuthHandler) setRefreshTokenCookie(
	c *gin.Context,
	token string,
) {
	duration := h.cfg.JWT.RefreshTokenDuration

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    token,
		Path:     refreshTokenCookiePath,
		MaxAge:   int(duration.Seconds()),
		Expires:  time.Now().UTC().Add(duration),
		HttpOnly: true,
		Secure:   h.refreshCookieSecure(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearRefreshTokenCookie(
	c *gin.Context,
) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    "",
		Path:     refreshTokenCookiePath,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   h.refreshCookieSecure(),
		SameSite: http.SameSiteLaxMode,
	})
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

		case errors.Is(err, repository.ErrEmailAlreadyUsed):
			response.Conflict(c, err.Error())

		// Defaults to an HTTP 500 Internal Server Error for unhandled exceptions.
		default:
			response.InternalServerError(c)
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
	h.setRefreshTokenCookie(
		c,
		result.RefreshToken,
	)

	response.OK(
		c,
		"Login successful",
		result.Public(),
	)
}

// Refresh handles token rotation when the user's short-lived Access Token expires.
func (h *AuthHandler) Refresh(c *gin.Context) {

	refreshToken, err := c.Cookie(
		refreshTokenCookieName,
	)
	if err != nil {
		response.Unauthorized(
			c,
			"Refresh token is required",
		)
		return
	}

	req := authservice.RefreshTokenRequest{
		RefreshToken: refreshToken,
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
	h.setRefreshTokenCookie(
		c,
		result.RefreshToken,
	)

	response.OK(
		c,
		"Token refreshed successfully",
		result.Public(),
	)
}

// Logout terminates user sessions safely.
func (h *AuthHandler) Logout(c *gin.Context) {

	refreshToken, err := c.Cookie(
		refreshTokenCookieName,
	)
	if err != nil {
		h.clearRefreshTokenCookie(c)

		response.OK(
			c,
			"Logout successful",
			nil,
		)

		return
	}

	req := authservice.RefreshTokenRequest{
		RefreshToken: refreshToken,
	}

	// Drops or invalidates the token record from the persistent database.
	if err := h.service.Logout(
		c.Request.Context(),
		req,
	); err != nil {
		response.InternalServerError(c)
		return
	}

	// Clear the browser refresh-token cookie after revocation.
	h.clearRefreshTokenCookie(c)

	// Returns an HTTP 200 OK status confirming complete session destruction.
	response.OK(
		c,
		"Logout successful",
		nil,
	)
}
