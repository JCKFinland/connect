package models

type ServiceCategory struct {
	BaseModel

	Code        string  `db:"code" json:"code"`
	Name        string  `db:"name" json:"name"`
	Description *string `db:"description" json:"description,omitempty"`
	IsActive    bool    `db:"is_active" json:"is_active"`
}
