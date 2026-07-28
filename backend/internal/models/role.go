package models

// Role represents a security role.
type Role struct {
	BaseModel

	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`
}
