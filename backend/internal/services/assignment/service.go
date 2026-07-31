package assignment

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
)

type Dependencies struct {
	Assignments repository.DriverAssignmentRepository
	Presence    *presence.Service
}

type Service struct {
	assignments repository.DriverAssignmentRepository
	presence    *presence.Service
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		assignments: deps.Assignments,
		presence:    deps.Presence,
	}
}