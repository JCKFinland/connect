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

	ErrInvalidPaymentOperation = errors.New(
		"invalid payment operation",
	)

	ErrPaymentOperationIdempotencyConflict = errors.New(
		"payment operation idempotency conflict",
	)

	ErrPaymentOperationAmountRequired = errors.New(
		"payment operation amount is required",
	)

	ErrPaymentOperationAmountInvalid = errors.New(
		"invalid payment operation amount",
	)
)
