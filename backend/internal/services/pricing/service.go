package pricing

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Service interface {
	Resolve(
		ctx context.Context,
		input ResolveInput,
	) (*ResolveResult, error)
}

type Dependencies struct {
	FarePricingProfiles repository.FarePricingProfileRepository
}

type service struct {
	farePricingProfiles repository.FarePricingProfileRepository
}

func NewService(
	deps Dependencies,
) Service {
	return &service{
		farePricingProfiles: deps.FarePricingProfiles,
	}
}
