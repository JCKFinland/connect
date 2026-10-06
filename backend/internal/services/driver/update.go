package driver

import (
	"context"
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Update modifies an existing driver.
func (s *Service) Update(
	ctx context.Context,
	userID string,
	id string,
	req UpdateDriverRequest,
) error {

	driver, err := s.resolveManagementTarget(
		ctx,
		userID,
		id,
	)
	if err != nil {
		return err
	}

	// Generic management update is intentionally profile-only. Tenant and
	// branch authority, regulatory credentials, verification metadata, and
	// lifecycle state are owned by their dedicated workflows.
	driver.DriverNumber = req.DriverNumber
	driver.FirstName = req.FirstName
	driver.LastName = req.LastName
	driver.Phone = req.Phone
	driver.Email = req.Email
	driver.HireDate = req.HireDate
	driver.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateDetails(
		ctx,
		driver,
	); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrDriverNotFound
		}

		return err
	}

	return nil
}
