package fleet

import "time"

type CreateFleetRequest struct {
	CompanyID  string `json:"company_id" binding:"required,uuid"`
	BranchID   string `json:"branch_id" binding:"required,uuid"`
	Code       string `json:"code" binding:"required,max=50"`
	Name       string `json:"name" binding:"required,max=255"`
	Description string `json:"description"`
	IsActive   bool   `json:"is_active"`
}

type UpdateFleetRequest struct {
	CompanyID  string `json:"company_id" binding:"required,uuid"`
	BranchID   string `json:"branch_id" binding:"required,uuid"`
	Code       string `json:"code" binding:"required,max=50"`
	Name       string `json:"name" binding:"required,max=255"`
	Description string `json:"description"`
	IsActive   bool   `json:"is_active"`
}

type FleetResponse struct {
	ID          string     `json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	CompanyID   string     `json:"company_id"`
	BranchID    string     `json:"branch_id"`

	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Description string     `json:"description"`

	IsActive    bool       `json:"is_active"`
}