package auth

// RegisterRequest represents a user registration request.
type RegisterRequest struct {
	Email     string `json:"email" validate:"required,email,max=255"`
	Password  string `json:"password" validate:"required,min=8,max=72"`
	FirstName string `json:"first_name" validate:"required,max=100"`
	LastName  string `json:"last_name" validate:"required,max=100"`
	Phone     string `json:"phone" validate:"required,max=30"`
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RefreshTokenRequest represents a refresh token request.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// AuthResponse is returned after a successful login.
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`

	TokenType string `json:"token_type"`

	ExpiresIn int64 `json:"expires_in"`
}

// UserResponse is a safe representation of a user.
type UserResponse struct {
	ID string `json:"id"`

	Email string `json:"email"`

	FirstName string `json:"first_name"`

	LastName string `json:"last_name"`

	Phone string `json:"phone"`

	IsVerified bool `json:"is_verified"`
}
