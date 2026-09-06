package stripe

import (
	"fmt"

	stripego "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func mapPaymentIntentStatus(
	transactionType string,
	status stripego.PaymentIntentStatus,
) (string, bool, error) {
	switch status {

	case stripego.PaymentIntentStatusSucceeded:
		return paymenttransaction.StatusSuccess,
			false,
			nil

	case stripego.PaymentIntentStatusProcessing:
		return paymenttransaction.StatusProcessing,
			false,
			nil

	case stripego.PaymentIntentStatusRequiresPaymentMethod,
		stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresAction:

		return paymenttransaction.StatusProcessing,
			true,
			nil

	case stripego.PaymentIntentStatusRequiresCapture:
		if transactionType ==
			paymenttransaction.TypeAuthorize {

			return paymenttransaction.StatusSuccess,
				false,
				nil
		}

		return "", false, fmt.Errorf(
			"%w: Stripe PaymentIntent requires capture for %s",
			ErrInvalidOperation,
			transactionType,
		)

	case stripego.PaymentIntentStatusCanceled:
		return paymenttransaction.StatusCancelled,
			false,
			nil

	default:
		return "", false, fmt.Errorf(
			"%w: unsupported Stripe PaymentIntent status %q",
			ErrInvalidOperation,
			status,
		)
	}
}
