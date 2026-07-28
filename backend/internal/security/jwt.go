package security

import (
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

// JWTService handles JWT generation and validation.
type JWTService struct {
	secretKey []byte
	issuer    string
	duration  time.Duration
}

// NewJWTService creates a new JWT service.
func NewJWTService(cfg *config.Config) *JWTService {
	return &JWTService{
		secretKey: []byte(cfg.JWT.Secret),
		issuer:    cfg.JWT.Issuer,
		duration:  cfg.JWT.AccessTokenDuration,
	}
}

// GenerateAccessToken generates a signed JWT access token.
func (s *JWTService) GenerateAccessToken(user *models.User) (string, error) {

	now := time.Now()

	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.duration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(s.secretKey)
}

// ValidateToken validates a JWT string.
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {

	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {

			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}

			return s.secretKey, nil
		},
	)

	if err != nil {

		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}

		return nil, err
	}

	claims, ok := token.Claims.(*Claims)

	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ParseClaims returns the JWT claims.
func (s *JWTService) ParseClaims(tokenString string) (*Claims, error) {
	return s.ValidateToken(tokenString)
}
