package pricing

import (
	"time"

	"github.com/JCKFinland/connect/backend/internal/services/fare"
)

type ResolveInput struct {
	CompanyID string

	BranchID *string

	ServiceCategoryID string

	At time.Time
}

type ResolveResult struct {
	ProfileID string

	ServiceCategoryID string

	Pricing fare.PricingSnapshot
}
