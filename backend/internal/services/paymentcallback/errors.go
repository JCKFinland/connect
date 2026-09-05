package paymentcallback

import "errors"

var (
	ErrInvalidCallback = errors.New(
		"invalid payment provider callback",
	)

	ErrCallbackTransactionNotFound = errors.New(
		"payment provider callback transaction not found",
	)

	ErrCallbackProviderMismatch = errors.New(
		"payment provider callback provider mismatch",
	)

	ErrCallbackVerificationFailed = errors.New(
		"payment provider callback verification failed",
	)

	ErrUnsupportedCallbackProvider = errors.New(
		"unsupported payment callback provider",
	)

	ErrCallbackVerifierAlreadyRegistered = errors.New(
		"payment callback verifier already registered",
	)
)
