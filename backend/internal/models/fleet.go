package models

// Fleet represents a group of vehicles belonging to a branch.
type Fleet struct {
	BaseModel
	SoftDelete

	CompanyID string `db:"company_id" json:"company_id"`

	BranchID string `db:"branch_id" json:"branch_id"`

	Name string `db:"name" json:"name"`

	Description string `db:"description" json:"description"`

	IsActive bool `db:"is_active" json:"is_active"`
}