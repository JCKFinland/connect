package paymenttransaction

import paymentservice "github.com/JCKFinland/connect/backend/internal/services/payment"

func canInitiateOperation(
	paymentStatus string,
	transactionType string,
) bool {
	switch paymentStatus {
	case paymentservice.StatusPending:
		return transactionType == TypeSale ||
			transactionType == TypeAuthorize

	case paymentservice.StatusAuthorized:
		return transactionType == TypeCapture ||
			transactionType == TypeVoid

	case paymentservice.StatusPaid,
		paymentservice.StatusPartiallyRefunded:

		return transactionType == TypeRefund

	default:
		return false
	}
}
