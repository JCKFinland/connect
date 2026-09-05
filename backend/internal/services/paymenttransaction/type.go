package paymenttransaction

const (
	TypeAuthorize = "AUTHORIZE"
	TypeCapture   = "CAPTURE"
	TypeSale      = "SALE"
	TypeRefund    = "REFUND"
	TypeVoid      = "VOID"
)

func isValidTransactionType(
	transactionType string,
) bool {
	switch transactionType {
	case TypeAuthorize,
		TypeCapture,
		TypeSale,
		TypeRefund,
		TypeVoid:

		return true

	default:
		return false
	}
}