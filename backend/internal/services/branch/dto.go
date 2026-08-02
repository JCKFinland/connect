package branch

type CreateBranchRequest struct {
	CompanyID string `json:"company_id" binding:"required"`

	Code string `json:"code" binding:"required"`

	Name string `json:"name" binding:"required"`

	Email string `json:"email"`

	Phone string `json:"phone"`

	AddressLine1 string `json:"address_line1"`

	AddressLine2 string `json:"address_line2"`

	City string `json:"city"`

	State string `json:"state"`

	PostalCode string `json:"postal_code"`

	Latitude float64 `json:"latitude"`

	Longitude float64 `json:"longitude"`

	IsActive bool `json:"is_active"`
}

type UpdateBranchRequest struct {
	Code string `json:"code"`

	Name string `json:"name"`

	Email string `json:"email"`

	Phone string `json:"phone"`

	AddressLine1 string `json:"address_line1"`

	AddressLine2 string `json:"address_line2"`

	City string `json:"city"`

	State string `json:"state"`

	PostalCode string `json:"postal_code"`

	Latitude float64 `json:"latitude"`

	Longitude float64 `json:"longitude"`

	IsActive bool `json:"is_active"`
}