package models

// Branch represents a physical branch of a company.
type Branch struct {
	BaseModel
	SoftDelete

	CompanyID string `db:"company_id" json:"company_id"`

	Name string `db:"name" json:"name"`

	Address string `db:"address" json:"address"`

	City string `db:"city" json:"city"`

	PostalCode string `db:"postal_code" json:"postal_code"`

	Phone string `db:"phone" json:"phone"`

	Email string `db:"email" json:"email"`

	IsActive bool `db:"is_active" json:"is_active"`
}