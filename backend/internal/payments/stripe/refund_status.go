package stripe

import (
	"fmt"

	stripego "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func mapRefundStatus(
	status stripego.RefundStatus,
) (string, bool, error) {
	switch status {
	case stripego.RefundStatusSucceeded:
		return paymenttransaction.StatusSuccess,
			false,
			nil

	case stripego.RefundStatusPending:
		return paymenttransaction.StatusProcessing,
			false,
			nil

	case stripego.RefundStatusRequiresAction:
		return paymenttransaction.StatusProcessing,
			true,
			nil

	case stripego.RefundStatusCanceled:
		return paymenttransaction.StatusCancelled,
			false,
			nil

	case stripego.RefundStatusFailed:
		return paymenttransaction.StatusFailed,
			false,
			nil

	default:
		return "", false, fmt.Errorf(
			"%w: unsupported Stripe refund status %q",
			ErrInvalidOperation,
			status,
		)
	}
}
