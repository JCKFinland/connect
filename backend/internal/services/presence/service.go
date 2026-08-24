package presence

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB     *pgxpool.Pool
	Config *config.Config

	Users       repository.UserRepository
	Drivers     repository.DriverRepository
	Presence    repository.DriverPresenceRepository
	Assignments repository.DriverAssignmentRepository
}

type Service struct {
	db  *pgxpool.Pool
	cfg *config.Config

	users       repository.UserRepository
	drivers     repository.DriverRepository
	presence    repository.DriverPresenceRepository
	assignments repository.DriverAssignmentRepository
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		db:  deps.DB,
		cfg: deps.Config,

		users:       deps.Users,
		drivers:     deps.Drivers,
		presence:    deps.Presence,
		assignments: deps.Assignments,
	}
}
