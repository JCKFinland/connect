package logger

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Middleware logs every HTTP request.
func Middleware(log *slog.Logger) gin.HandlerFunc {

	return func(c *gin.Context) {

		start := time.Now()

		requestID := uuid.NewString()

		c.Set("request_id", requestID)

		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Next()

		duration := time.Since(start)

		log.Info(
			"HTTP Request",
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.String("client_ip", c.ClientIP()),
			slog.Duration("duration", duration),
			slog.String("user_agent", c.Request.UserAgent()),
		)
	}
}