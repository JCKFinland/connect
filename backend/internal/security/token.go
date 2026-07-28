package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// TokenPair represents an access token and refresh token.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// GenerateRefreshToken creates a cryptographically secure random refresh token.
func GenerateRefreshToken() (string, error) {

	// 32 bytes = 256 bits
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashRefreshToken hashes the refresh token before storing it.
func HashRefreshToken(token string) string {

	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}

// GenerateTokenPair creates both an access token and a refresh token.
func GenerateTokenPair(
	jwtService *JWTService,
	user *models.User,
) (*TokenPair, error) {

	accessToken, err := jwtService.GenerateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
