package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/config"
)

func newAuthCookieTestHandler(
	env string,
) *AuthHandler {
	return NewAuthHandler(
		nil,
		&config.Config{
			App: config.AppConfig{
				Env: env,
			},
			JWT: config.JWTConfig{
				RefreshTokenDuration: 30 * 24 * time.Hour,
			},
		},
	)
}

func TestSetRefreshTokenCookieDevelopment(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	handler := newAuthCookieTestHandler(
		"development",
	)

	handler.setRefreshTokenCookie(
		c,
		"secret-refresh-token",
	)

	cookies := recorder.Result().Cookies()

	if len(cookies) != 1 {
		t.Fatalf(
			"expected 1 cookie, got %d",
			len(cookies),
		)
	}

	cookie := cookies[0]

	if cookie.Name != refreshTokenCookieName {
		t.Fatalf(
			"unexpected cookie name %q",
			cookie.Name,
		)
	}

	if cookie.Value != "secret-refresh-token" {
		t.Fatalf(
			"unexpected cookie value %q",
			cookie.Value,
		)
	}

	if cookie.Path != refreshTokenCookiePath {
		t.Fatalf(
			"unexpected cookie path %q",
			cookie.Path,
		)
	}

	if !cookie.HttpOnly {
		t.Fatal("expected refresh cookie to be HttpOnly")
	}

	if cookie.Secure {
		t.Fatal(
			"development refresh cookie must not require HTTPS",
		)
	}

	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf(
			"expected SameSite=Lax, got %v",
			cookie.SameSite,
		)
	}

	expectedMaxAge := int(
		(30 * 24 * time.Hour).Seconds(),
	)

	if cookie.MaxAge != expectedMaxAge {
		t.Fatalf(
			"expected MaxAge %d, got %d",
			expectedMaxAge,
			cookie.MaxAge,
		)
	}
}

func TestSetRefreshTokenCookieProductionIsSecure(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	handler := newAuthCookieTestHandler(
		"production",
	)

	handler.setRefreshTokenCookie(
		c,
		"secret-refresh-token",
	)

	cookies := recorder.Result().Cookies()

	if len(cookies) != 1 {
		t.Fatalf(
			"expected 1 cookie, got %d",
			len(cookies),
		)
	}

	if !cookies[0].Secure {
		t.Fatal(
			"production refresh cookie must be Secure",
		)
	}
}

func TestClearRefreshTokenCookie(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	handler := newAuthCookieTestHandler(
		"development",
	)

	handler.clearRefreshTokenCookie(c)

	cookies := recorder.Result().Cookies()

	if len(cookies) != 1 {
		t.Fatalf(
			"expected 1 cookie, got %d",
			len(cookies),
		)
	}

	cookie := cookies[0]

	if cookie.Name != refreshTokenCookieName {
		t.Fatalf(
			"unexpected cookie name %q",
			cookie.Name,
		)
	}

	if cookie.Value != "" {
		t.Fatalf(
			"expected empty cookie value, got %q",
			cookie.Value,
		)
	}

	if cookie.MaxAge != -1 {
		t.Fatalf(
			"expected MaxAge -1, got %d",
			cookie.MaxAge,
		)
	}

	if !cookie.HttpOnly {
		t.Fatal(
			"cleared refresh cookie must remain HttpOnly",
		)
	}
}

func TestRefreshWithoutCookieReturnsUnauthorized(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/refresh",
		nil,
	)

	handler := newAuthCookieTestHandler(
		"development",
	)

	handler.Refresh(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestLogoutWithoutCookieIsIdempotent(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/logout",
		nil,
	)

	handler := newAuthCookieTestHandler(
		"development",
	)

	handler.Logout(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	cookies := recorder.Result().Cookies()

	if len(cookies) != 1 {
		t.Fatalf(
			"expected logout to clear 1 cookie, got %d",
			len(cookies),
		)
	}

	cookie := cookies[0]

	if cookie.Name != refreshTokenCookieName {
		t.Fatalf(
			"unexpected cookie name %q",
			cookie.Name,
		)
	}

	if cookie.MaxAge != -1 {
		t.Fatalf(
			"expected cleared cookie MaxAge -1, got %d",
			cookie.MaxAge,
		)
	}
}
