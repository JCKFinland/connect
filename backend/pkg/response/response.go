package response

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Meta struct {
	RequestID string    `json:"request_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    Meta        `json:"meta"`
}

func Success(
	c *gin.Context,
	status int,
	message string,
	data interface{},
) {
	requestID, _ := c.Get("request_id")

	c.JSON(status, SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
		Meta: Meta{
			RequestID: toString(requestID),
			Timestamp: time.Now().UTC(),
		},
	})
}

func OK(c *gin.Context, message string, data interface{}) {
	Success(c, http.StatusOK, message, data)
}

func Created(c *gin.Context, message string, data interface{}) {
	Success(c, http.StatusCreated, message, data)
}
