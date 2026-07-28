package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID generates a unique request ID for every incoming request.
// The ID is stored in the Gin context and returned in the X-Request-ID header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {

		requestID := uuid.NewString()

		c.Set("request_id", requestID)

		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Next()
	}
}
