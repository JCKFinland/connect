package models

// ServiceCategory represents a commercial ride or service product
// offered by a company.
//
// It is intentionally separate from vehicles.vehicle_type.
// Vehicle type describes the physical vehicle; service category
// describes the product that the customer requests.
type ServiceCategory struct {
	BaseModel

	CompanyID string `db:"company_id" json:"company_id"`

	Code        string  `db:"code" json:"code"`
	Name        string  `db:"name" json:"name"`
	Description *string `db:"description" json:"description,omitempty"`

	IsActive bool `db:"is_active" json:"is_active"`
}
