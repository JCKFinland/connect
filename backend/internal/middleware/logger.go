package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger logs every HTTP request.
func Logger(log *slog.Logger) gin.HandlerFunc {

	return func(c *gin.Context) {

		start := time.Now()

		c.Next()

		requestID, _ := c.Get("request_id")

		log.Info(
			"HTTP Request",
			slog.String("request_id", toString(requestID)),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.String("client_ip", c.ClientIP()),
			slog.Duration("duration", time.Since(start)),
			slog.String("user_agent", c.Request.UserAgent()),
		)
	}
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}