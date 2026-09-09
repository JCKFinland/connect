package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	stripego "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

var (
	ErrInvalidOperation = errors.New(
		"invalid Stripe payment operation",
	)

	ErrUnsupportedOperation = errors.New(
		"unsupported Stripe payment operation",
	)
)

// ExecuteRequest contains the already-persisted CONNECT financial
// operation that must be submitted to Stripe.
//
// The PaymentTransaction remains authoritative for amount, currency,
// transaction type, reference, and idempotency identity.
type ExecuteRequest struct {
	Transaction *models.PaymentTransaction

	// ParentProviderTransactionID is the previously persisted Stripe
	// provider identity required by operations that act on an existing
	// PaymentIntent.
	//
	// Required for:
	//   CAPTURE
	//   REFUND
	//   VOID
	//
	// Not required for:
	//   SALE
	//   AUTHORIZE
	ParentProviderTransactionID string
}

// ExecuteResult represents the immediate provider-side state returned
// after submitting an operation to Stripe.
//
// Final asynchronous state may subsequently arrive through the
// verified Stripe webhook boundary.
type ExecuteResult struct {
	ProviderTransactionID string

	Status string

	// ClientSecret is an ephemeral Stripe credential used by the
	// customer application to continue PaymentIntent confirmation.
	//
	// It must not be logged or persisted.
	ClientSecret string

	RequiresCustomerAction bool
}

type Executor interface {
	Execute(
		ctx context.Context,
		req ExecuteRequest,
	) (*ExecuteResult, error)
}

type executor struct {
	paymentIntents paymentIntentClient

	refunds refundClient
}

func NewExecutor(
	secretKey string,
) (Executor, error) {
	secretKey = strings.TrimSpace(secretKey)

	if secretKey == "" {
		return nil, fmt.Errorf(
			"%w: Stripe secret key is required",
			ErrInvalidOperation,
		)
	}

	client := stripego.NewClient(secretKey)

	return &executor{
		paymentIntents: client.V1PaymentIntents,

		refunds: client.V1Refunds,
	}, nil
}

func newExecutorWithPaymentIntents(
	client paymentIntentClient,
) Executor {
	return &executor{
		paymentIntents: client,
	}
}

func newExecutorWithClients(
	paymentIntents paymentIntentClient,
	refunds refundClient,
) Executor {
	return &executor{
		paymentIntents: paymentIntents,

		refunds: refunds,
	}
}

func (e *executor) Execute(
	ctx context.Context,
	req ExecuteRequest,
) (*ExecuteResult, error) {
	if req.Transaction == nil {
		return nil, fmt.Errorf(
			"%w: payment transaction is required",
			ErrInvalidOperation,
		)
	}

	if e.paymentIntents == nil {
		return nil, errors.New(
			"Stripe PaymentIntent client is not configured",
		)
	}

	transaction := req.Transaction

	if strings.TrimSpace(transaction.ID) == "" {
		return nil, fmt.Errorf(
			"%w: transaction ID is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(transaction.PaymentID) == "" {
		return nil, fmt.Errorf(
			"%w: payment ID is required",
			ErrInvalidOperation,
		)
	}

	if !strings.EqualFold(
		strings.TrimSpace(transaction.Provider),
		ProviderName,
	) {
		return nil, fmt.Errorf(
			"%w: provider must be %s",
			ErrInvalidOperation,
			ProviderName,
		)
	}

	if transaction.IdempotencyKey == nil ||
		strings.TrimSpace(*transaction.IdempotencyKey) == "" {

		return nil, fmt.Errorf(
			"%w: idempotency key is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(transaction.Amount) == "" {
		return nil, fmt.Errorf(
			"%w: amount is required",
			ErrInvalidOperation,
		)
	}

	if strings.TrimSpace(transaction.Currency) == "" {
		return nil, fmt.Errorf(
			"%w: currency is required",
			ErrInvalidOperation,
		)
	}

	switch transaction.TransactionType {
	case paymenttransaction.TypeSale,
		paymenttransaction.TypeAuthorize:

		return e.createPaymentIntent(
			ctx,
			transaction,
		)

	case paymenttransaction.TypeCapture:
		if strings.TrimSpace(
			req.ParentProviderTransactionID,
		) == "" {
			return nil, fmt.Errorf(
				"%w: parent provider transaction ID is required for %s",
				ErrInvalidOperation,
				transaction.TransactionType,
			)
		}

		return e.capturePaymentIntent(
			ctx,
			transaction,
			req.ParentProviderTransactionID,
		)

	case paymenttransaction.TypeVoid:
		if strings.TrimSpace(
			req.ParentProviderTransactionID,
		) == "" {
			return nil, fmt.Errorf(
				"%w: parent provider transaction ID is required for %s",
				ErrInvalidOperation,
				transaction.TransactionType,
			)
		}

		return e.cancelPaymentIntent(
			ctx,
			transaction,
			req.ParentProviderTransactionID,
		)

	case paymenttransaction.TypeRefund:
		if strings.TrimSpace(
			req.ParentProviderTransactionID,
		) == "" {
			return nil, fmt.Errorf(
				"%w: parent provider transaction ID is required for %s",
				ErrInvalidOperation,
				transaction.TransactionType,
			)
		}

		return e.createRefund(
			ctx,
			transaction,
			req.ParentProviderTransactionID,
		)

	default:
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedOperation,
			transaction.TransactionType,
		)
	}
}

func (e *executor) createPaymentIntent(
	ctx context.Context,
	transaction *models.PaymentTransaction,
) (*ExecuteResult, error) {
	amount, err :=
		amountToMinorUnits(
			transaction.Amount,
			transaction.Currency,
		)
	if err != nil {
		return nil, err
	}

	currency :=
		strings.ToLower(
			strings.TrimSpace(
				transaction.Currency,
			),
		)

	captureMethod :=
		string(
			stripego.PaymentIntentCaptureMethodAutomatic,
		)

	if transaction.TransactionType ==
		paymenttransaction.TypeAuthorize {

		captureMethod =
			string(
				stripego.PaymentIntentCaptureMethodManual,
			)
	}

	automaticPaymentMethodsEnabled := true

	description :=
		"CONNECT payment " +
			transaction.PaymentID

	params :=
		&stripego.PaymentIntentCreateParams{
			Amount: &amount,

			Currency: &currency,

			CaptureMethod: &captureMethod,

			AutomaticPaymentMethods: &stripego.PaymentIntentCreateAutomaticPaymentMethodsParams{
				Enabled: &automaticPaymentMethodsEnabled,
			},

			Description: &description,

			Metadata: map[string]string{
				"connect_payment_id": transaction.PaymentID,

				"connect_transaction_id": transaction.ID,

				"connect_transaction_reference": transaction.TransactionReference,

				"connect_transaction_type": transaction.TransactionType,
			},
		}

	params.SetIdempotencyKey(
		strings.TrimSpace(
			*transaction.IdempotencyKey,
		),
	)

	intent, err :=
		e.paymentIntents.Create(
			ctx,
			params,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"create Stripe PaymentIntent: %w",
			err,
		)
	}

	if intent == nil {
		return nil, errors.New(
			"Stripe PaymentIntent create returned nil result",
		)
	}

	if strings.TrimSpace(intent.ID) == "" {
		return nil, errors.New(
			"Stripe PaymentIntent create returned empty ID",
		)
	}

	status, requiresCustomerAction, err :=
		mapPaymentIntentStatus(
			transaction.TransactionType,
			intent.Status,
		)
	if err != nil {
		return nil, err
	}

	return &ExecuteResult{
		ProviderTransactionID: intent.ID,

		Status: status,

		ClientSecret: intent.ClientSecret,

		RequiresCustomerAction: requiresCustomerAction,
	}, nil
}

func (e *executor) capturePaymentIntent(
	ctx context.Context,
	transaction *models.PaymentTransaction,
	parentProviderTransactionID string,
) (*ExecuteResult, error) {
	amount, err :=
		amountToMinorUnits(
			transaction.Amount,
			transaction.Currency,
		)
	if err != nil {
		return nil, err
	}

	params :=
		&stripego.PaymentIntentCaptureParams{
			AmountToCapture: &amount,

			Metadata: map[string]string{
				"connect_payment_id": transaction.PaymentID,

				"connect_transaction_id": transaction.ID,

				"connect_transaction_reference": transaction.TransactionReference,

				"connect_transaction_type": transaction.TransactionType,
			},
		}

	params.SetIdempotencyKey(
		strings.TrimSpace(
			*transaction.IdempotencyKey,
		),
	)

	intent, err :=
		e.paymentIntents.Capture(
			ctx,
			strings.TrimSpace(
				parentProviderTransactionID,
			),
			params,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"capture Stripe PaymentIntent: %w",
			err,
		)
	}

	if intent == nil {
		return nil, errors.New(
			"Stripe PaymentIntent capture returned nil result",
		)
	}

	if strings.TrimSpace(intent.ID) == "" {
		return nil, errors.New(
			"Stripe PaymentIntent capture returned empty ID",
		)
	}

	status, requiresCustomerAction, err :=
		mapPaymentIntentStatus(
			transaction.TransactionType,
			intent.Status,
		)
	if err != nil {
		return nil, err
	}

	return &ExecuteResult{
		ProviderTransactionID: intent.ID,

		Status: status,

		ClientSecret: intent.ClientSecret,

		RequiresCustomerAction: requiresCustomerAction,
	}, nil
}

func (e *executor) cancelPaymentIntent(
	ctx context.Context,
	transaction *models.PaymentTransaction,
	parentProviderTransactionID string,
) (*ExecuteResult, error) {
	paymentIntentID :=
		strings.TrimSpace(
			parentProviderTransactionID,
		)

	updateParams :=
		&stripego.PaymentIntentUpdateParams{
			Metadata: map[string]string{
				"connect_payment_id": transaction.PaymentID,

				"connect_transaction_id": transaction.ID,

				"connect_transaction_reference": transaction.TransactionReference,

				"connect_transaction_type": transaction.TransactionType,
			},
		}

	if _, err := e.paymentIntents.Update(
		ctx,
		paymentIntentID,
		updateParams,
	); err != nil {
		return nil, fmt.Errorf(
			"update Stripe PaymentIntent VOID metadata: %w",
			err,
		)
	}

	params :=
		&stripego.PaymentIntentCancelParams{}

	params.SetIdempotencyKey(
		strings.TrimSpace(
			*transaction.IdempotencyKey,
		),
	)

	intent, err :=
		e.paymentIntents.Cancel(
			ctx,
			paymentIntentID,
			params,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"cancel Stripe PaymentIntent: %w",
			err,
		)
	}

	if intent == nil {
		return nil, errors.New(
			"Stripe PaymentIntent cancel returned nil result",
		)
	}

	if strings.TrimSpace(intent.ID) == "" {
		return nil, errors.New(
			"Stripe PaymentIntent cancel returned empty ID",
		)
	}

	status, requiresCustomerAction, err :=
		mapPaymentIntentStatus(
			transaction.TransactionType,
			intent.Status,
		)
	if err != nil {
		return nil, err
	}

	return &ExecuteResult{
		ProviderTransactionID: intent.ID,

		Status: status,

		ClientSecret: intent.ClientSecret,

		RequiresCustomerAction: requiresCustomerAction,
	}, nil
}

func (e *executor) createRefund(
	ctx context.Context,
	transaction *models.PaymentTransaction,
	parentProviderTransactionID string,
) (*ExecuteResult, error) {
	if e.refunds == nil {
		return nil, errors.New(
			"Stripe Refund client is not configured",
		)
	}

	amount, err :=
		amountToMinorUnits(
			transaction.Amount,
			transaction.Currency,
		)
	if err != nil {
		return nil, err
	}

	paymentIntentID :=
		strings.TrimSpace(
			parentProviderTransactionID,
		)

	params :=
		&stripego.RefundCreateParams{
			Amount: &amount,

			PaymentIntent: &paymentIntentID,

			Metadata: map[string]string{
				"connect_payment_id": transaction.PaymentID,

				"connect_transaction_id": transaction.ID,

				"connect_transaction_reference": transaction.TransactionReference,

				"connect_transaction_type": transaction.TransactionType,
			},
		}

	params.SetIdempotencyKey(
		strings.TrimSpace(
			*transaction.IdempotencyKey,
		),
	)

	refund, err :=
		e.refunds.Create(
			ctx,
			params,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"create Stripe refund: %w",
			err,
		)
	}

	if refund == nil {
		return nil, errors.New(
			"Stripe refund create returned nil result",
		)
	}

	if strings.TrimSpace(refund.ID) == "" {
		return nil, errors.New(
			"Stripe refund create returned empty ID",
		)
	}

	status, requiresCustomerAction, err :=
		mapRefundStatus(
			refund.Status,
		)
	if err != nil {
		return nil, err
	}

	return &ExecuteResult{
		ProviderTransactionID: refund.ID,

		Status: status,

		RequiresCustomerAction: requiresCustomerAction,
	}, nil
}

var _ Executor = (*executor)(nil)
