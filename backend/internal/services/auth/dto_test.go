package auth

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthResponsePublicExcludesRefreshToken(
	t *testing.T,
) {
	result := &AuthResponse{
		AccessToken:  "access-token",
		RefreshToken: "secret-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    900,
	}

	payload, err := json.Marshal(result.Public())
	if err != nil {
		t.Fatalf(
			"marshal public auth response: %v",
			err,
		)
	}

	body := string(payload)

	if strings.Contains(
		body,
		"secret-refresh-token",
	) {
		t.Fatal(
			"public auth response exposed refresh token",
		)
	}

	if strings.Contains(
		body,
		"refresh_token",
	) {
		t.Fatal(
			"public auth response contains refresh_token field",
		)
	}

	if !strings.Contains(
		body,
		`"access_token":"access-token"`,
	) {
		t.Fatal(
			"public auth response is missing access token",
		)
	}
}
