package models

// Company represents a transport operator registered in CONNECT.
type Company struct {
	BaseModel
	SoftDelete

	Name string `db:"name" json:"name"`

	RegistrationNumber string `db:"registration_number" json:"registration_number"`

	TaxNumber string `db:"tax_number" json:"tax_number"`

	Email string `db:"email" json:"email"`

	Phone string `db:"phone" json:"phone"`

	Address string `db:"address" json:"address"`

	City string `db:"city" json:"city"`

	PostalCode string `db:"postal_code" json:"postal_code"`

	Country string `db:"country" json:"country"`

	IsActive bool `db:"is_active" json:"is_active"`
}