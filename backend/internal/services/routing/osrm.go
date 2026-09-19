package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHTTPTimeout = 10 * time.Second

type osrmService struct {
	baseURL string
	client  *http.Client
}

type osrmResponse struct {
	Code   string      `json:"code"`
	Routes []osrmRoute `json:"routes"`
}

type osrmRoute struct {
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
}

// NewOSRMService creates an OSRM-compatible road-routing service.
func NewOSRMService(
	baseURL string,
	client *http.Client,
) Service {
	if client == nil {
		client = &http.Client{
			Timeout: defaultHTTPTimeout,
		}
	}

	return &osrmService{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (s *osrmService) Route(
	ctx context.Context,
	request RouteRequest,
) (*RouteMetrics, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}

	endpoint, err := s.routeURL(request)
	if err != nil {
		return nil, ErrRouteUnavailable
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return nil, ErrRouteUnavailable
	}

	response, err := s.client.Do(httpRequest)
	if err != nil {
		return nil, ErrRouteUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, ErrRouteUnavailable
	}

	var payload osrmResponse

	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, ErrRouteUnavailable
	}

	if payload.Code != "Ok" || len(payload.Routes) == 0 {
		return nil, ErrRouteUnavailable
	}

	route := payload.Routes[0]

	if route.Distance < 0 ||
		route.Duration < 0 ||
		math.IsNaN(route.Distance) ||
		math.IsNaN(route.Duration) ||
		math.IsInf(route.Distance, 0) ||
		math.IsInf(route.Duration, 0) {
		return nil, ErrRouteUnavailable
	}

	return &RouteMetrics{
		DistanceMeters: int64(math.Round(route.Distance)),
		DurationSeconds: int64(
			math.Round(route.Duration),
		),
	}, nil
}

func (s *osrmService) routeURL(
	request RouteRequest,
) (string, error) {
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse routing base URL: %w", err)
	}

	coordinates := fmt.Sprintf(
		"%.6f,%.6f;%.6f,%.6f",
		request.Origin.Longitude,
		request.Origin.Latitude,
		request.Destination.Longitude,
		request.Destination.Latitude,
	)

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") +
		"/route/v1/driving/" +
		coordinates

	query := baseURL.Query()
	query.Set("overview", "false")
	query.Set("alternatives", "false")
	query.Set("steps", "false")
	baseURL.RawQuery = query.Encode()

	return baseURL.String(), nil
}
