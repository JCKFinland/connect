package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/pkg/response"
)

// Recovery catches panics, logs them, and returns a standard JSON response.
func Recovery(log *slog.Logger) gin.HandlerFunc {

	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {

		requestID, _ := c.Get("request_id")

		log.Error(
			"panic recovered",
			slog.Any("panic", recovered),
			slog.String("request_id", toString(requestID)),
		)

		response.Error(
			c,
			http.StatusInternalServerError,
			"An unexpected internal server error occurred.",
			nil,
		)

		c.Abort()
	})
}