package routing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOSRMServiceRouteReturnsRoadMetrics(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			if request.Method != http.MethodGet {
				t.Fatalf(
					"method = %s, want GET",
					request.Method,
				)
			}

			if !strings.Contains(
				request.URL.Path,
				"/route/v1/driving/24.655900,60.205500;24.941400,60.171900",
			) {
				t.Fatalf(
					"unexpected route path %q",
					request.URL.Path,
				)
			}

			response.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = response.Write([]byte(`{
				"code":"Ok",
				"routes":[{
					"distance":22134.6,
					"duration":1682.4
				}]
			}`))
		}),
	)
	defer server.Close()

	service := NewOSRMService(
		server.URL,
		server.Client(),
	)

	metrics, err := service.Route(
		context.Background(),
		RouteRequest{
			Origin: Coordinate{
				Latitude:  60.2055,
				Longitude: 24.6559,
			},
			Destination: Coordinate{
				Latitude:  60.1719,
				Longitude: 24.9414,
			},
		},
	)
	if err != nil {
		t.Fatalf("Route() unexpected error: %v", err)
	}

	if metrics.DistanceMeters != 22135 {
		t.Fatalf(
			"distance = %d, want 22135",
			metrics.DistanceMeters,
		)
	}

	if metrics.DurationSeconds != 1682 {
		t.Fatalf(
			"duration = %d, want 1682",
			metrics.DurationSeconds,
		)
	}
}

func TestOSRMServiceRouteRejectsProviderFailure(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			response http.ResponseWriter,
			_ *http.Request,
		) {
			response.WriteHeader(
				http.StatusServiceUnavailable,
			)
		}),
	)
	defer server.Close()

	service := NewOSRMService(
		server.URL,
		server.Client(),
	)

	_, err := service.Route(
		context.Background(),
		RouteRequest{
			Origin: Coordinate{
				Latitude:  60.2055,
				Longitude: 24.6559,
			},
			Destination: Coordinate{
				Latitude:  60.1719,
				Longitude: 24.9414,
			},
		},
	)

	if err != ErrRouteUnavailable {
		t.Fatalf(
			"Route() error = %v, want %v",
			err,
			ErrRouteUnavailable,
		)
	}
}

func TestOSRMServiceRouteRejectsNoRoute(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			response http.ResponseWriter,
			_ *http.Request,
		) {
			response.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = response.Write([]byte(`{
				"code":"NoRoute",
				"routes":[]
			}`))
		}),
	)
	defer server.Close()

	service := NewOSRMService(
		server.URL,
		server.Client(),
	)

	_, err := service.Route(
		context.Background(),
		RouteRequest{
			Origin: Coordinate{
				Latitude:  60.2055,
				Longitude: 24.6559,
			},
			Destination: Coordinate{
				Latitude:  60.1719,
				Longitude: 24.9414,
			},
		},
	)

	if err != ErrRouteUnavailable {
		t.Fatalf(
			"Route() error = %v, want %v",
			err,
			ErrRouteUnavailable,
		)
	}
}

func TestOSRMServiceRouteRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			response http.ResponseWriter,
			_ *http.Request,
		) {
			response.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = response.Write([]byte(`{invalid-json`))
		}),
	)
	defer server.Close()

	service := NewOSRMService(
		server.URL,
		server.Client(),
	)

	_, err := service.Route(
		context.Background(),
		RouteRequest{
			Origin: Coordinate{
				Latitude:  60.2055,
				Longitude: 24.6559,
			},
			Destination: Coordinate{
				Latitude:  60.1719,
				Longitude: 24.9414,
			},
		},
	)

	if err != ErrRouteUnavailable {
		t.Fatalf(
			"Route() error = %v, want %v",
			err,
			ErrRouteUnavailable,
		)
	}
}

func TestOSRMServiceRouteHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})

	server := httptest.NewServer(
		http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			close(started)

			<-request.Context().Done()

			response.WriteHeader(
				http.StatusServiceUnavailable,
			)
		}),
	)
	defer server.Close()

	service := NewOSRMService(
		server.URL,
		server.Client(),
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	result := make(chan error, 1)

	go func() {
		_, err := service.Route(
			ctx,
			RouteRequest{
				Origin: Coordinate{
					Latitude:  60.2055,
					Longitude: 24.6559,
				},
				Destination: Coordinate{
					Latitude:  60.1719,
					Longitude: 24.9414,
				},
			},
		)

		result <- err
	}()

	<-started
	cancel()

	err := <-result

	if err != ErrRouteUnavailable {
		t.Fatalf(
			"Route() error = %v, want %v",
			err,
			ErrRouteUnavailable,
		)
	}
}
