package security

import "github.com/golang-jwt/jwt/v5"

// Claims represents the JWT payload.
type Claims struct {
	UserID string `json:"user_id"`

	Email string `json:"email"`

	jwt.RegisteredClaims
}
