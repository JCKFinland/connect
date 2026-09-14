package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newAuthValidationTestContext(
	t *testing.T,
	method string,
	path string,
	body string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		method,
		path,
		strings.NewReader(body),
	)

	c.Request.Header.Set(
		"Content-Type",
		"application/json",
	)

	return c, recorder
}

func TestRegisterRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing email",
			body: `{
				"password":"password123",
				"first_name":"John",
				"last_name":"Doe",
				"phone":"+358401234567"
			}`,
		},
		{
			name: "malformed email",
			body: `{
				"email":"not-an-email",
				"password":"password123",
				"first_name":"John",
				"last_name":"Doe",
				"phone":"+358401234567"
			}`,
		},
		{
			name: "password shorter than eight characters",
			body: `{
				"email":"john@example.com",
				"password":"short",
				"first_name":"John",
				"last_name":"Doe",
				"phone":"+358401234567"
			}`,
		},
		{
			name: "password longer than seventy two characters",
			body: `{
				"email":"john@example.com",
				"password":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"first_name":"John",
				"last_name":"Doe",
				"phone":"+358401234567"
			}`,
		},
		{
			name: "missing first name",
			body: `{
				"email":"john@example.com",
				"password":"password123",
				"last_name":"Doe",
				"phone":"+358401234567"
			}`,
		},
		{
			name: "missing last name",
			body: `{
				"email":"john@example.com",
				"password":"password123",
				"first_name":"John",
				"phone":"+358401234567"
			}`,
		},
		{
			name: "missing phone",
			body: `{
				"email":"john@example.com",
				"password":"password123",
				"first_name":"John",
				"last_name":"Doe"
			}`,
		},
	}

	handler := NewAuthHandler(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newAuthValidationTestContext(
				t,
				http.MethodPost,
				"/api/v1/auth/register",
				tt.body,
			)

			handler.Register(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					http.StatusBadRequest,
					recorder.Code,
					recorder.Body.String(),
				)
			}
		})
	}
}

func TestLoginRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed email",
			body: `{
				"email":"not-an-email",
				"password":"password123"
			}`,
		},
		{
			name: "missing password",
			body: `{
				"email":"john@example.com"
			}`,
		},
	}

	handler := NewAuthHandler(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newAuthValidationTestContext(
				t,
				http.MethodPost,
				"/api/v1/auth/login",
				tt.body,
			)

			handler.Login(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					http.StatusBadRequest,
					recorder.Code,
					recorder.Body.String(),
				)
			}
		})
	}
}

func TestRefreshRejectsMissingRefreshToken(
	t *testing.T,
) {
	handler := NewAuthHandler(nil)

	c, recorder := newAuthValidationTestContext(
		t,
		http.MethodPost,
		"/api/v1/auth/refresh",
		`{}`,
	)

	handler.Refresh(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestLogoutRejectsMissingRefreshToken(
	t *testing.T,
) {
	handler := NewAuthHandler(nil)

	c, recorder := newAuthValidationTestContext(
		t,
		http.MethodPost,
		"/api/v1/auth/logout",
		`{}`,
	)

	handler.Logout(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}
