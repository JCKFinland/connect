package models

import "time"

// BaseModel contains common fields shared by most entities.
type BaseModel struct {
	ID        string    `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// SoftDelete adds soft-delete capability.
type SoftDelete struct {
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}
