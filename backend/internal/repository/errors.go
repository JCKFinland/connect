package repository

import "errors"

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrEmailAlreadyUsed    = errors.New("email already exists")
	ErrInvalidCredential   = errors.New("invalid credentials")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrAccountDisabled     = errors.New("account is disabled")
	ErrNotFound            = errors.New("resource not found")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
)
