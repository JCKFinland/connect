package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	fareestimateservice "github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
	"github.com/JCKFinland/connect/backend/internal/services/routing"
)

type fareEstimateServiceStub struct {
	request fareestimateservice.EstimateRequest
	result  *fareestimateservice.EstimateResult
	err     error
}

func (s *fareEstimateServiceStub) Estimate(
	_ context.Context,
	request fareestimateservice.EstimateRequest,
) (*fareestimateservice.EstimateResult, error) {
	s.request = request

	return s.result, s.err
}

func TestFareEstimateHandlerEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &fareEstimateServiceStub{
		result: &fareestimateservice.EstimateResult{
			DistanceMeters:   8420,
			DurationSeconds:  900,
			BaseFare:         4.90,
			DistanceFare:     12.63,
			TimeFare:         3.75,
			BookingFee:       2.00,
			SurgeAmount:      0,
			TotalAmount:      23.28,
			Currency:         "EUR",
			PricingProfileID: "profile-1",
			PricingVersion:   "v1",
		},
	}

	handler := NewFareEstimateHandler(service)

	router := gin.New()
	router.POST("/fare-estimates", handler.Estimate)

	body := `{
		"pickup_latitude": 60.2055,
		"pickup_longitude": 24.6559,
		"destination_latitude": 60.1719,
		"destination_longitude": 24.9414,
		"service_category_id": "category-1"
	}`

	request := httptest.NewRequest(
		http.MethodPost,
		"/fare-estimates",
		bytes.NewBufferString(body),
	)
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			recorder.Code,
			http.StatusOK,
			recorder.Body.String(),
		)
	}

	if service.request.ServiceCategoryID != "category-1" {
		t.Fatalf(
			"service category = %q, want category-1",
			service.request.ServiceCategoryID,
		)
	}

	if service.request.PickupLatitude != 60.2055 ||
		service.request.PickupLongitude != 24.6559 {
		t.Fatalf(
			"unexpected pickup coordinates: %+v",
			service.request,
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"total_amount":23.28`,
	) {
		t.Fatalf(
			"response missing total amount: %s",
			recorder.Body.String(),
		)
	}
}

func TestFareEstimateHandlerRejectsInvalidRequest(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	service := &fareEstimateServiceStub{}

	handler := NewFareEstimateHandler(service)

	router := gin.New()
	router.POST("/fare-estimates", handler.Estimate)

	request := httptest.NewRequest(
		http.MethodPost,
		"/fare-estimates",
		bytes.NewBufferString(`{}`),
	)
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			recorder.Code,
			http.StatusBadRequest,
			recorder.Body.String(),
		)
	}
}

func TestFareEstimateHandlerPricingUnavailable(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	service := &fareEstimateServiceStub{
		err: pricing.ErrPricingProfileNotFound,
	}

	handler := NewFareEstimateHandler(service)

	router := gin.New()
	router.POST("/fare-estimates", handler.Estimate)

	body := `{
		"pickup_latitude": 60.2055,
		"pickup_longitude": 24.6559,
		"destination_latitude": 60.1719,
		"destination_longitude": 24.9414,
		"service_category_id": "category-1"
	}`

	request := httptest.NewRequest(
		http.MethodPost,
		"/fare-estimates",
		bytes.NewBufferString(body),
	)
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			recorder.Code,
			http.StatusUnprocessableEntity,
			recorder.Body.String(),
		)
	}
}

func TestFareEstimateHandlerRouteUnavailable(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	service := &fareEstimateServiceStub{
		err: routing.ErrRouteUnavailable,
	}

	handler := NewFareEstimateHandler(service)

	router := gin.New()
	router.POST("/fare-estimates", handler.Estimate)

	body := `{
		"pickup_latitude": 60.2055,
		"pickup_longitude": 24.6559,
		"destination_latitude": 60.1719,
		"destination_longitude": 24.9414,
		"service_category_id": "category-1"
	}`

	request := httptest.NewRequest(
		http.MethodPost,
		"/fare-estimates",
		bytes.NewBufferString(body),
	)
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			recorder.Code,
			http.StatusServiceUnavailable,
			recorder.Body.String(),
		)
	}
}

func TestFareEstimateHandlerInternalError(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	service := &fareEstimateServiceStub{
		err: errors.New("unexpected failure"),
	}

	handler := NewFareEstimateHandler(service)

	router := gin.New()
	router.POST("/fare-estimates", handler.Estimate)

	body := `{
		"pickup_latitude": 60.2055,
		"pickup_longitude": 24.6559,
		"destination_latitude": 60.1719,
		"destination_longitude": 24.9414,
		"service_category_id": "category-1"
	}`

	request := httptest.NewRequest(
		http.MethodPost,
		"/fare-estimates",
		bytes.NewBufferString(body),
	)
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"status = %d, want %d; body=%s",
			recorder.Code,
			http.StatusInternalServerError,
			recorder.Body.String(),
		)
	}
}
