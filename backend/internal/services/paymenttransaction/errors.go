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

	ErrUnsupportedTransactionType = errors.New(
		"unsupported payment transaction type",
	)

	ErrRefundExceedsPaymentAmount = errors.New(
		"successful refunds exceed payment amount",
	)
)
