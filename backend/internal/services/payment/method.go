package payment

const (
	MethodPI           = "PI"
	MethodCard         = "CARD"
	MethodCash         = "CASH"
	MethodBankTransfer = "BANK_TRANSFER"
	MethodWallet       = "WALLET"
)

func isValidPaymentMethod(
	method string,
) bool {
	switch method {
	case MethodPI,
		MethodCard,
		MethodCash,
		MethodBankTransfer,
		MethodWallet:

		return true

	default:
		return false
	}
}
