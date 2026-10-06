package branch

import "errors"

var (
	ErrBranchHasFleets = errors.New(
		"branch contains non-archived fleets",
	)
	ErrBranchHasActiveFleets = errors.New(
		"branch cannot be deactivated while it contains active fleets",
	)

	ErrBranchNotFound = errors.New("branch not found")
)
