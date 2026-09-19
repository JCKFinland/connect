package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	fareestimateservice "github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
	"github.com/JCKFinland/connect/backend/internal/services/routing"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type FareEstimateHandler struct {
	service fareestimateservice.Service
}

func NewFareEstimateHandler(
	service fareestimateservice.Service,
) *FareEstimateHandler {
	return &FareEstimateHandler{
		service: service,
	}
}

type fareEstimateRequest struct {
	PickupLatitude float64 `json:"pickup_latitude" binding:"required,gte=-90,lte=90"`

	PickupLongitude float64 `json:"pickup_longitude" binding:"required,gte=-180,lte=180"`

	DestinationLatitude float64 `json:"destination_latitude" binding:"required,gte=-90,lte=90"`

	DestinationLongitude float64 `json:"destination_longitude" binding:"required,gte=-180,lte=180"`

	ServiceCategoryID string `json:"service_category_id" binding:"required"`
}

func (h *FareEstimateHandler) Estimate(
	c *gin.Context,
) {
	var request fareEstimateRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(
			c,
			"Invalid fare estimate request",
		)
		return
	}

	result, err := h.service.Estimate(
		c.Request.Context(),
		fareestimateservice.EstimateRequest{
			PickupLatitude: request.PickupLatitude,

			PickupLongitude: request.PickupLongitude,

			DestinationLatitude: request.DestinationLatitude,

			DestinationLongitude: request.DestinationLongitude,

			ServiceCategoryID: request.ServiceCategoryID,
		},
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			fareestimateservice.ErrInvalidServiceCategoryID,
		):
			response.BadRequest(
				c,
				"Invalid service category",
			)

		case errors.Is(
			err,
			routing.ErrInvalidOrigin,
		),
			errors.Is(
				err,
				routing.ErrInvalidDestination,
			):
			response.BadRequest(
				c,
				"Invalid route coordinates",
			)

		case errors.Is(
			err,
			routing.ErrRouteUnavailable,
		):
			response.Error(
				c,
				http.StatusServiceUnavailable,
				"Route estimate unavailable",
				nil,
			)

		case errors.Is(
			err,
			pricing.ErrPricingProfileNotFound,
		):
			response.UnprocessableEntity(
				c,
				"Fare pricing is unavailable for this service category",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.OK(
		c,
		"Fare estimate calculated",
		result,
	)
}
