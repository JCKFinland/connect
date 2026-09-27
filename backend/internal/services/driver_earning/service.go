package driver_earning

import "github.com/JCKFinland/connect/backend/internal/repository"

const recentEarningsLimit = 50

type Dependencies struct {
	Earnings repository.DriverEarningRepository
}

type Service struct {
	earnings repository.DriverEarningRepository
}

func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		earnings: deps.Earnings,
	}
}
