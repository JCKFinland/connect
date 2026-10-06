package company

import "errors"

var (
	ErrCompanyNotFound = errors.New(
		"company not found",
	)
	ErrCompanyAlreadyExists = errors.New(
		"company already exists",
	)
	ErrCompanyCreationAccessDenied = errors.New(
		"company creation access denied",
	)
	ErrCompanyHasBranches = errors.New(
		"company contains non-archived branches",
	)
	ErrCompanyHasActiveBranches = errors.New(
		"company cannot be deactivated while it contains active branches",
	)
)
