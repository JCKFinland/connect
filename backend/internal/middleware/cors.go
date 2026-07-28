package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS configures Cross-Origin Resource Sharing.
func CORS() gin.HandlerFunc {

	return cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:5173", // Customer App
			"http://localhost:5174", // Driver App
			"http://localhost:5175", // Admin Portal
		},

		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},

		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
			"X-Request-ID",
		},

		ExposeHeaders: []string{
			"X-Request-ID",
		},

		AllowCredentials: true,

		MaxAge: 12 * time.Hour,
	})
}
