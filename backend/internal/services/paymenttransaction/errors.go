package paymenttransaction

import "errors"

var (
	ErrInvalidTransactionStatus = errors.New(
		"invalid payment transaction status",
	)

	ErrInvalidTransactionTransition = errors.New(
		"invalid payment transaction status transition",
	)

	ErrProviderIdentityConflict = errors.New(
		"payment transaction provider identity conflict",
	)
)
