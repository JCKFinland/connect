package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Errors  []ValidationError `json:"errors,omitempty"`
	Meta    Meta              `json:"meta"`
}

func Error(
	c *gin.Context,
	status int,
	message string,
	errors []ValidationError,
) {
	requestID, _ := c.Get("request_id")

	c.JSON(status, ErrorResponse{
		Success: false,
		Message: message,
		Errors:  errors,
		Meta: Meta{
			RequestID: toString(requestID),
			Timestamp: time.Now().UTC(),
		},
	})
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message, nil)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, message, nil)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, message, nil)
}

func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, message, nil)
}

func InternalServerError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, message, nil)
}