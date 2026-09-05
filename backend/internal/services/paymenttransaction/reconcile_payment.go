package paymenttransaction

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	paymentservice "github.com/JCKFinland/connect/backend/internal/services/payment"
)

func reconcileSuccessfulPaymentOperation(
	ctx context.Context,
	payments *postgresrepo.PaymentRepository,
	transactions *postgresrepo.PaymentTransactionRepository,
	currentPayment *models.Payment,
	currentTransaction *models.PaymentTransaction,
) error {
	if currentTransaction.Status != StatusSuccess {
		return nil
	}

	var targetStatus string

	switch currentTransaction.TransactionType {
	case TypeSale,
		TypeCapture:

		targetStatus = paymentservice.StatusPaid

	case TypeAuthorize:

		targetStatus = paymentservice.StatusAuthorized

	case TypeVoid:

		targetStatus = paymentservice.StatusCancelled

	case TypeRefund:
		refundState, err :=
			transactions.GetSuccessfulRefundState(
				ctx,
				currentPayment.ID,
			)
		if err != nil {
			return fmt.Errorf(
				"determine aggregate refund state: %w",
				err,
			)
		}

		switch refundState {
		case repository.SuccessfulRefundPartial:
			targetStatus =
				paymentservice.StatusPartiallyRefunded

		case repository.SuccessfulRefundFull:
			targetStatus =
				paymentservice.StatusRefunded

		case repository.SuccessfulRefundOver:
			return ErrRefundExceedsPaymentAmount

		case repository.SuccessfulRefundNone:
			return nil

		default:
			return fmt.Errorf(
				"unknown successful refund state: %s",
				refundState,
			)
		}

	default:
		return fmt.Errorf(
			"%w: %s",
			ErrUnsupportedTransactionType,
			currentTransaction.TransactionType,
		)
	}

	if currentPayment.Status == targetStatus {
		return nil
	}

	if !paymentservice.CanTransitionStatus(
		currentPayment.Status,
		targetStatus,
	) {
		return fmt.Errorf(
			"%w: %s -> %s",
			paymentservice.ErrInvalidPaymentTransition,
			currentPayment.Status,
			targetStatus,
		)
	}

	if err := payments.UpdateStatus(
		ctx,
		currentPayment.ID,
		targetStatus,
	); err != nil {
		return fmt.Errorf(
			"persist aggregate payment status: %w",
			err,
		)
	}

	return nil
}