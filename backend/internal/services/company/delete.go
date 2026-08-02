package company

import (
	"context"
)

func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {

	_, err := s.companies.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return err
	}

	return s.companies.Delete(
		ctx,
		id,
	)
}