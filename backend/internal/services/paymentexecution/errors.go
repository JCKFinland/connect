package paymentexecution

import "errors"

var (
	ErrUnsupportedProvider = errors.New(
		"unsupported payment execution provider",
	)

	ErrParentProviderTransactionNotFound = errors.New(
		"parent provider transaction not found",
	)
)
