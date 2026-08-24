package assignment

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB *pgxpool.Pool

	Assignments repository.DriverAssignmentRepository
	Presence    *presence.Service
}

type Service struct {
	db *pgxpool.Pool

	assignments repository.DriverAssignmentRepository
	presence    *presence.Service
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		db: deps.DB,

		assignments: deps.Assignments,
		presence:    deps.Presence,
	}
}
