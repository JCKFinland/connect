package company

type CreateCompanyRequest struct {
	Name string `json:"name" binding:"required"`

	LegalName string `json:"legal_name"`

	BusinessID string `json:"business_id"`

	Email string `json:"email"`

	Phone string `json:"phone"`

	Website string `json:"website"`

	CountryCode string `json:"country_code"`

	Timezone string `json:"timezone"`

	AddressLine1 string `json:"address_line1"`

	AddressLine2 string `json:"address_line2"`

	City string `json:"city"`

	State string `json:"state"`

	PostalCode string `json:"postal_code"`

	LogoURL string `json:"logo_url"`

	IsActive bool `json:"is_active"`
}

type UpdateCompanyRequest struct {
	Name string `json:"name"`

	LegalName string `json:"legal_name"`

	BusinessID string `json:"business_id"`

	Email string `json:"email"`

	Phone string `json:"phone"`

	Website string `json:"website"`

	CountryCode string `json:"country_code"`

	Timezone string `json:"timezone"`

	AddressLine1 string `json:"address_line1"`

	AddressLine2 string `json:"address_line2"`

	City string `json:"city"`

	State string `json:"state"`

	PostalCode string `json:"postal_code"`

	LogoURL string `json:"logo_url"`

	IsActive bool `json:"is_active"`
}