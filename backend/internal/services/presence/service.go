package presence

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Dependencies struct {
	Config *config.Config

	Users repository.UserRepository

	Presence repository.DriverPresenceRepository
}

type Service struct {
	cfg *config.Config

	users repository.UserRepository

	presence repository.DriverPresenceRepository
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		cfg: deps.Config,
		users: deps.Users,
		presence: deps.Presence,
	}
}