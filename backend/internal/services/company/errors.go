package company

import "errors"

var (
	ErrCompanyNotFound = errors.New(
		"company not found",
	)

	ErrCompanyAlreadyExists = errors.New(
		"company already exists",
	)
)