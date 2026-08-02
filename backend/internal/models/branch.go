package models


// Branch represents a physical branch of a company.
type Branch struct {
	BaseModel
	SoftDelete

	CompanyID string `db:"company_id" json:"company_id"`

	Code string `db:"code" json:"code"`

	Name string `db:"name" json:"name"`

	Email string `db:"email" json:"email"`

	Phone string `db:"phone" json:"phone"`

	AddressLine1 string `db:"address_line1" json:"address_line1"`

	AddressLine2 string `db:"address_line2" json:"address_line2"`

	City string `db:"city" json:"city"`

	State string `db:"state" json:"state"`

	PostalCode string `db:"postal_code" json:"postal_code"`

	Latitude float64 `db:"latitude" json:"latitude"`

	Longitude float64 `db:"longitude" json:"longitude"`

	IsActive bool `db:"is_active" json:"is_active"`
}