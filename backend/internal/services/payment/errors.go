package payment

import "errors"

var (
	ErrInvalidPaymentMethod = errors.New(
		"invalid payment method",
	)

	ErrInvalidPaymentStatus = errors.New(
		"invalid payment status",
	)

	ErrInvalidPaymentTransition = errors.New(
		"invalid payment status transition",
	)

	ErrPaymentAccessDenied = errors.New(
		"payment access denied",
	)

	ErrPaymentAlreadyExists = errors.New(
		"payment already exists",
	)
)
