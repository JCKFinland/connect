package models

// Company represents a transport operator registered in CONNECT.
type Company struct {
	BaseModel
	SoftDelete

	Name string `db:"name" json:"name"`

	LegalName string `db:"legal_name" json:"legal_name"`

	BusinessID string `db:"business_id" json:"business_id"`

	Email string `db:"email" json:"email"`

	Phone string `db:"phone" json:"phone"`

	Website string `db:"website" json:"website"`

	CountryCode string `db:"country_code" json:"country_code"`

	Timezone string `db:"timezone" json:"timezone"`

	AddressLine1 string `db:"address_line1" json:"address_line1"`

	AddressLine2 string `db:"address_line2" json:"address_line2"`

	City string `db:"city" json:"city"`

	State string `db:"state" json:"state"`

	PostalCode string `db:"postal_code" json:"postal_code"`

	LogoURL string `db:"logo_url" json:"logo_url"`

	IsActive bool `db:"is_active" json:"is_active"`
}