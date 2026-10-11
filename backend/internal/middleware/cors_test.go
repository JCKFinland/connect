package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSDevelopmentOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		origin string
		want   int
	}{
		{"customer localhost", "http://localhost:5173", http.StatusNoContent},
		{"customer loopback", "http://127.0.0.1:5173", http.StatusNoContent},
		{"driver localhost", "http://localhost:5174", http.StatusNoContent},
		{"driver loopback", "http://127.0.0.1:5174", http.StatusNoContent},
		{"admin localhost", "http://localhost:5175", http.StatusNoContent},
		{"admin loopback", "http://127.0.0.1:5175", http.StatusNoContent},
		{"unknown origin", "http://127.0.0.1:5999", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORS())
			router.POST("/api/v1/auth/login", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			request := httptest.NewRequest(
				http.MethodOptions,
				"/api/v1/auth/login",
				nil,
			)
			request.Header.Set("Origin", tt.origin)
			request.Header.Set("Access-Control-Request-Method", "POST")
			request.Header.Set("Access-Control-Request-Headers", "content-type")

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.want {
				t.Fatalf(
					"status = %d, want %d",
					recorder.Code,
					tt.want,
				)
			}

			if tt.want == http.StatusForbidden {
				if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
					t.Fatalf("unexpected allowed origin: %q", got)
				}
				return
			}

			if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != tt.origin {
				t.Errorf("allowed origin = %q, want %q", got, tt.origin)
			}

			if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Errorf("allow credentials = %q, want true", got)
			}

			if got := recorder.Header().Get("Access-Control-Allow-Methods"); got == "" {
				t.Error("missing allowed methods")
			}

			if got := recorder.Header().Get("Access-Control-Allow-Headers"); got == "" {
				t.Error("missing allowed headers")
			}
		})
	}
}
