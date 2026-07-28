package models

// User represents a CONNECT user.
type User struct {
	BaseModel
	SoftDelete

	Email        string `db:"email" json:"email"`
	PasswordHash string `db:"password_hash" json:"-"`

	FirstName string `db:"first_name" json:"first_name"`
	LastName  string `db:"last_name" json:"last_name"`

	Phone string `db:"phone" json:"phone"`

	IsActive   bool `db:"is_active" json:"is_active"`
	IsVerified bool `db:"is_verified" json:"is_verified"`
}
