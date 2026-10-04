package models

import "time"

// CompanyMembership represents explicit authority for a user to act
// within a company tenant.
//
// Roles and permissions define what a user may do. CompanyMembership
// defines which company tenant the user may act within.
type CompanyMembership struct {
	UserID    string    `db:"user_id" json:"user_id"`
	CompanyID string    `db:"company_id" json:"company_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
