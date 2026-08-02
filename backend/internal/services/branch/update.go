package branch

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Update(
	ctx context.Context,
	id string,
	req UpdateBranchRequest,
) error {

	branch, err := s.branches.GetByID(ctx, id)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrBranchNotFound
		}
		return err
	}

	branch.Code = req.Code
	branch.Name = req.Name
	branch.Email = req.Email
	branch.Phone = req.Phone
	branch.AddressLine1 = req.AddressLine1
	branch.AddressLine2 = req.AddressLine2
	branch.City = req.City
	branch.State = req.State
	branch.PostalCode = req.PostalCode
	branch.Latitude = req.Latitude
	branch.Longitude = req.Longitude
	branch.IsActive = req.IsActive

	return s.branches.Update(ctx, branch)
}