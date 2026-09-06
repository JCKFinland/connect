package stripe

import (
	"context"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

type recordingPaymentIntentClient struct {
	create func(
		ctx context.Context,
		params *stripego.PaymentIntentCreateParams,
	) (*stripego.PaymentIntent, error)

	capture func(
		ctx context.Context,
		id string,
		params *stripego.PaymentIntentCaptureParams,
	) (*stripego.PaymentIntent, error)

	cancel func(
		ctx context.Context,
		id string,
		params *stripego.PaymentIntentCancelParams,
	) (*stripego.PaymentIntent, error)
}

func (c *recordingPaymentIntentClient) Create(
	ctx context.Context,
	params *stripego.PaymentIntentCreateParams,
) (*stripego.PaymentIntent, error) {
	if c.create == nil {
		panic(
			"unexpected PaymentIntent Create call",
		)
	}

	return c.create(
		ctx,
		params,
	)
}

func (c *recordingPaymentIntentClient) Capture(
	ctx context.Context,
	id string,
	params *stripego.PaymentIntentCaptureParams,
) (*stripego.PaymentIntent, error) {
	if c.capture == nil {
		panic(
			"unexpected PaymentIntent Capture call",
		)
	}

	return c.capture(
		ctx,
		id,
		params,
	)
}

func (c *recordingPaymentIntentClient) Cancel(
	ctx context.Context,
	id string,
	params *stripego.PaymentIntentCancelParams,
) (*stripego.PaymentIntent, error) {
	if c.cancel == nil {
		panic(
			"unexpected PaymentIntent Cancel call",
		)
	}

	return c.cancel(
		ctx,
		id,
		params,
	)
}

type recordingRefundClient struct {
	create func(
		ctx context.Context,
		params *stripego.RefundCreateParams,
	) (*stripego.Refund, error)
}

func (c *recordingRefundClient) Create(
	ctx context.Context,
	params *stripego.RefundCreateParams,
) (*stripego.Refund, error) {
	if c.create == nil {
		panic(
			"unexpected Refund Create call",
		)
	}

	return c.create(
		ctx,
		params,
	)
}

func TestExecutorCreatesSalePaymentIntent(
	t *testing.T,
) {
	idempotencyKey :=
		"connect-sale-idempotency"

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-sale-123",
			},

			PaymentID: "payment-sale-123",

			TransactionReference: "txn_sale_123",

			Provider: ProviderName,

			IdempotencyKey: &idempotencyKey,

			TransactionType: paymenttransaction.TypeSale,

			Amount: "23.45",

			Currency: "EUR",
		}

	client :=
		&recordingPaymentIntentClient{
			create: func(
				_ context.Context,
				params *stripego.PaymentIntentCreateParams,
			) (*stripego.PaymentIntent, error) {
				assertCreatePaymentIntentParams(
					t,
					params,
					transaction,
					2345,
					stripego.PaymentIntentCaptureMethodAutomatic,
				)

				return &stripego.PaymentIntent{
					ID: "pi_sale_123",

					ClientSecret: "pi_sale_123_secret_test",

					Status: stripego.PaymentIntentStatusRequiresPaymentMethod,

					CaptureMethod: stripego.PaymentIntentCaptureMethodAutomatic,
				}, nil
			},
		}

	executor :=
		newExecutorWithPaymentIntents(
			client,
		)

	result, err :=
		executor.Execute(
			context.Background(),
			ExecuteRequest{
				Transaction: transaction,
			},
		)
	if err != nil {
		t.Fatalf(
			"execute Stripe SALE: %v",
			err,
		)
	}

	if result.ProviderTransactionID !=
		"pi_sale_123" {

		t.Fatalf(
			"provider transaction ID got %q",
			result.ProviderTransactionID,
		)
	}

	if result.Status !=
		paymenttransaction.StatusProcessing {

		t.Fatalf(
			"status got %q want %q",
			result.Status,
			paymenttransaction.StatusProcessing,
		)
	}

	if result.ClientSecret !=
		"pi_sale_123_secret_test" {

		t.Fatalf(
			"client secret got %q",
			result.ClientSecret,
		)
	}

	if !result.RequiresCustomerAction {
		t.Fatal(
			"expected SALE to require customer continuation",
		)
	}
}

func TestExecutorCreatesAuthorizePaymentIntent(
	t *testing.T,
) {
	idempotencyKey :=
		"connect-authorize-idempotency"

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-authorize-123",
			},

			PaymentID: "payment-authorize-123",

			TransactionReference: "txn_authorize_123",

			Provider: ProviderName,

			IdempotencyKey: &idempotencyKey,

			TransactionType: paymenttransaction.TypeAuthorize,

			Amount: "23.45",

			Currency: "EUR",
		}

	client :=
		&recordingPaymentIntentClient{
			create: func(
				_ context.Context,
				params *stripego.PaymentIntentCreateParams,
			) (*stripego.PaymentIntent, error) {
				assertCreatePaymentIntentParams(
					t,
					params,
					transaction,
					2345,
					stripego.PaymentIntentCaptureMethodManual,
				)

				return &stripego.PaymentIntent{
					ID: "pi_authorize_123",

					ClientSecret: "pi_authorize_123_secret_test",

					Status: stripego.PaymentIntentStatusRequiresPaymentMethod,

					CaptureMethod: stripego.PaymentIntentCaptureMethodManual,
				}, nil
			},
		}

	executor :=
		newExecutorWithPaymentIntents(
			client,
		)

	result, err :=
		executor.Execute(
			context.Background(),
			ExecuteRequest{
				Transaction: transaction,
			},
		)
	if err != nil {
		t.Fatalf(
			"execute Stripe AUTHORIZE: %v",
			err,
		)
	}

	if result.ProviderTransactionID !=
		"pi_authorize_123" {

		t.Fatalf(
			"provider transaction ID got %q",
			result.ProviderTransactionID,
		)
	}

	if result.Status !=
		paymenttransaction.StatusProcessing {

		t.Fatalf(
			"status got %q want %q",
			result.Status,
			paymenttransaction.StatusProcessing,
		)
	}

	if result.ClientSecret !=
		"pi_authorize_123_secret_test" {

		t.Fatalf(
			"client secret got %q",
			result.ClientSecret,
		)
	}

	if !result.RequiresCustomerAction {
		t.Fatal(
			"expected AUTHORIZE to require customer continuation",
		)
	}
}

func TestExecutorCapturesAuthorizedPaymentIntent(
	t *testing.T,
) {
	idempotencyKey := "connect-capture-idempotency"

	transaction := &models.PaymentTransaction{
		BaseModel: models.BaseModel{
			ID: "transaction-capture-123",
		},
		PaymentID:            "payment-capture-123",
		TransactionReference: "txn_capture_123",
		Provider:             ProviderName,
		IdempotencyKey:       &idempotencyKey,
		TransactionType:      paymenttransaction.TypeCapture,
		Amount:               "23.45",
		Currency:             "EUR",
	}

	const parentProviderTransactionID = "pi_authorize_parent_123"

	client := &recordingPaymentIntentClient{
		capture: func(
			_ context.Context,
			id string,
			params *stripego.PaymentIntentCaptureParams,
		) (*stripego.PaymentIntent, error) {
			if id != parentProviderTransactionID {
				t.Fatalf(
					"capture PaymentIntent ID got %q want %q",
					id,
					parentProviderTransactionID,
				)
			}

			if params == nil {
				t.Fatal("expected PaymentIntent capture params")
			}

			if params.AmountToCapture == nil ||
				*params.AmountToCapture != 2345 {
				t.Fatalf(
					"amount to capture got %v want 2345",
					params.AmountToCapture,
				)
			}

			if params.IdempotencyKey == nil ||
				*params.IdempotencyKey != idempotencyKey {
				t.Fatalf(
					"idempotency key got %v want %q",
					params.IdempotencyKey,
					idempotencyKey,
				)
			}

			if params.Metadata["connect_payment_id"] != transaction.PaymentID {
				t.Fatal("CONNECT payment ID metadata missing")
			}

			if params.Metadata["connect_transaction_id"] != transaction.ID {
				t.Fatal("CONNECT transaction ID metadata missing")
			}

			if params.Metadata["connect_transaction_reference"] !=
				transaction.TransactionReference {
				t.Fatal("CONNECT transaction reference metadata missing")
			}

			if params.Metadata["connect_transaction_type"] !=
				transaction.TransactionType {
				t.Fatal("CONNECT transaction type metadata missing")
			}

			return &stripego.PaymentIntent{
				ID:     parentProviderTransactionID,
				Status: stripego.PaymentIntentStatusSucceeded,
			}, nil
		},
	}

	executor := newExecutorWithPaymentIntents(client)

	result, err := executor.Execute(
		context.Background(),
		ExecuteRequest{
			Transaction:                 transaction,
			ParentProviderTransactionID: parentProviderTransactionID,
		},
	)
	if err != nil {
		t.Fatalf("execute Stripe CAPTURE: %v", err)
	}

	if result == nil {
		t.Fatal("expected CAPTURE execution result")
	}

	if result.ProviderTransactionID != parentProviderTransactionID {
		t.Fatalf(
			"provider transaction ID got %q want %q",
			result.ProviderTransactionID,
			parentProviderTransactionID,
		)
	}

	if result.Status != paymenttransaction.StatusSuccess {
		t.Fatalf(
			"status got %q want %q",
			result.Status,
			paymenttransaction.StatusSuccess,
		)
	}

	if result.ClientSecret != "" {
		t.Fatalf(
			"CAPTURE client secret got %q want empty",
			result.ClientSecret,
		)
	}

	if result.RequiresCustomerAction {
		t.Fatal("successful CAPTURE must not require customer action")
	}
}

func TestExecutorVoidsAuthorizedPaymentIntent(
	t *testing.T,
) {
	idempotencyKey :=
		"connect-void-idempotency"

	transaction :=
		&models.PaymentTransaction{
			BaseModel: models.BaseModel{
				ID: "transaction-void-123",
			},

			PaymentID: "payment-void-123",

			TransactionReference: "txn_void_123",

			Provider: ProviderName,

			IdempotencyKey: &idempotencyKey,

			TransactionType: paymenttransaction.TypeVoid,

			Amount: "23.45",

			Currency: "EUR",
		}

	const parentProviderTransactionID = "pi_authorize_void_parent_123"

	client :=
		&recordingPaymentIntentClient{
			cancel: func(
				_ context.Context,
				id string,
				params *stripego.PaymentIntentCancelParams,
			) (*stripego.PaymentIntent, error) {
				if id != parentProviderTransactionID {
					t.Fatalf(
						"cancel PaymentIntent ID got %q want %q",
						id,
						parentProviderTransactionID,
					)
				}

				if params == nil {
					t.Fatal(
						"expected PaymentIntent cancel params",
					)
				}

				if params.IdempotencyKey == nil ||
					*params.IdempotencyKey != idempotencyKey {

					t.Fatalf(
						"idempotency key got %v want %q",
						params.IdempotencyKey,
						idempotencyKey,
					)
				}

				return &stripego.PaymentIntent{
					ID: parentProviderTransactionID,

					Status: stripego.PaymentIntentStatusCanceled,
				}, nil
			},
		}

	executor :=
		newExecutorWithPaymentIntents(
			client,
		)

	result, err :=
		executor.Execute(
			context.Background(),
			ExecuteRequest{
				Transaction: transaction,

				ParentProviderTransactionID: parentProviderTransactionID,
			},
		)
	if err != nil {
		t.Fatalf(
			"execute Stripe VOID: %v",
			err,
		)
	}

	if result == nil {
		t.Fatal(
			"expected VOID execution result",
		)
	}

	if result.ProviderTransactionID !=
		parentProviderTransactionID {

		t.Fatalf(
			"provider transaction ID got %q want %q",
			result.ProviderTransactionID,
			parentProviderTransactionID,
		)
	}

	if result.Status !=
		paymenttransaction.StatusCancelled {

		t.Fatalf(
			"status got %q want %q",
			result.Status,
			paymenttransaction.StatusCancelled,
		)
	}

	if result.ClientSecret != "" {
		t.Fatalf(
			"VOID client secret got %q want empty",
			result.ClientSecret,
		)
	}

	if result.RequiresCustomerAction {
		t.Fatal(
			"successful VOID must not require customer action",
		)
	}
}

func TestExecutorRefundsCapturedPaymentIntent(
	t *testing.T,
) {
	idempotencyKey := "connect-refund-idempotency"

	transaction := &models.PaymentTransaction{
		BaseModel: models.BaseModel{
			ID: "transaction-refund-123",
		},
		PaymentID:            "payment-refund-123",
		TransactionReference: "txn_refund_123",
		Provider:             ProviderName,
		IdempotencyKey:       &idempotencyKey,
		TransactionType:      paymenttransaction.TypeRefund,
		Amount:               "23.45",
		Currency:             "EUR",
	}

	const parentProviderTransactionID = "pi_refund_parent_123"

	refundClient := &recordingRefundClient{
		create: func(
			_ context.Context,
			params *stripego.RefundCreateParams,
		) (*stripego.Refund, error) {
			if params == nil {
				t.Fatal("expected Stripe refund create params")
			}

			if params.PaymentIntent == nil ||
				*params.PaymentIntent != parentProviderTransactionID {
				t.Fatalf(
					"payment intent got %v want %q",
					params.PaymentIntent,
					parentProviderTransactionID,
				)
			}

			if params.Amount == nil ||
				*params.Amount != 2345 {
				t.Fatalf(
					"refund amount got %v want 2345",
					params.Amount,
				)
			}

			if params.IdempotencyKey == nil ||
				*params.IdempotencyKey != idempotencyKey {
				t.Fatalf(
					"idempotency key got %v want %q",
					params.IdempotencyKey,
					idempotencyKey,
				)
			}

			if params.Metadata["connect_payment_id"] != transaction.PaymentID {
				t.Fatal("CONNECT payment ID metadata missing")
			}

			if params.Metadata["connect_transaction_id"] != transaction.ID {
				t.Fatal("CONNECT transaction ID metadata missing")
			}

			if params.Metadata["connect_transaction_reference"] !=
				transaction.TransactionReference {
				t.Fatal("CONNECT transaction reference metadata missing")
			}

			if params.Metadata["connect_transaction_type"] !=
				transaction.TransactionType {
				t.Fatal("CONNECT transaction type metadata missing")
			}

			return &stripego.Refund{
				ID:     "re_refund_123",
				Status: stripego.RefundStatusSucceeded,
			}, nil
		},
	}

	executor := newExecutorWithClients(
		&recordingPaymentIntentClient{},
		refundClient,
	)

	result, err := executor.Execute(
		context.Background(),
		ExecuteRequest{
			Transaction:                 transaction,
			ParentProviderTransactionID: parentProviderTransactionID,
		},
	)
	if err != nil {
		t.Fatalf("execute Stripe REFUND: %v", err)
	}

	if result == nil {
		t.Fatal("expected REFUND execution result")
	}

	if result.ProviderTransactionID != "re_refund_123" {
		t.Fatalf(
			"provider transaction ID got %q want %q",
			result.ProviderTransactionID,
			"re_refund_123",
		)
	}

	if result.Status != paymenttransaction.StatusSuccess {
		t.Fatalf(
			"status got %q want %q",
			result.Status,
			paymenttransaction.StatusSuccess,
		)
	}

	if result.ClientSecret != "" {
		t.Fatalf(
			"REFUND client secret got %q want empty",
			result.ClientSecret,
		)
	}

	if result.RequiresCustomerAction {
		t.Fatal("successful REFUND must not require customer action")
	}
}

func TestMapPaymentIntentStatusAuthorizeRequiresCapture(
	t *testing.T,
) {
	status, requiresAction, err :=
		mapPaymentIntentStatus(
			paymenttransaction.TypeAuthorize,
			stripego.PaymentIntentStatusRequiresCapture,
		)
	if err != nil {
		t.Fatalf(
			"map requires_capture: %v",
			err,
		)
	}

	if status !=
		paymenttransaction.StatusSuccess {

		t.Fatalf(
			"got %q want SUCCESS",
			status,
		)
	}

	if requiresAction {
		t.Fatal(
			"requires_capture must not require customer action",
		)
	}
}

func assertCreatePaymentIntentParams(
	t *testing.T,
	params *stripego.PaymentIntentCreateParams,
	transaction *models.PaymentTransaction,
	wantAmount int64,
	wantCaptureMethod stripego.PaymentIntentCaptureMethod,
) {
	t.Helper()

	if params == nil {
		t.Fatal(
			"expected PaymentIntent create params",
		)
	}

	if params.Amount == nil ||
		*params.Amount != wantAmount {

		t.Fatalf(
			"amount got %v want %d",
			params.Amount,
			wantAmount,
		)
	}

	if params.Currency == nil ||
		*params.Currency != "eur" {

		t.Fatalf(
			"currency got %v want eur",
			params.Currency,
		)
	}

	if params.CaptureMethod == nil ||
		*params.CaptureMethod !=
			string(wantCaptureMethod) {

		t.Fatalf(
			"capture method got %v want %s",
			params.CaptureMethod,
			wantCaptureMethod,
		)
	}

	if params.Confirm != nil &&
		*params.Confirm {

		t.Fatal(
			"CONNECT must not server-confirm the initial PaymentIntent",
		)
	}

	if params.AutomaticPaymentMethods == nil ||
		params.AutomaticPaymentMethods.Enabled == nil ||
		!*params.AutomaticPaymentMethods.Enabled {

		t.Fatal(
			"automatic payment methods must be enabled",
		)
	}

	if params.Metadata["connect_payment_id"] !=
		transaction.PaymentID {

		t.Fatal(
			"CONNECT payment ID metadata missing",
		)
	}

	if params.Metadata["connect_transaction_id"] !=
		transaction.ID {

		t.Fatal(
			"CONNECT transaction ID metadata missing",
		)
	}

	if params.Metadata["connect_transaction_reference"] !=
		transaction.TransactionReference {

		t.Fatal(
			"CONNECT transaction reference metadata missing",
		)
	}

	if params.Metadata["connect_transaction_type"] !=
		transaction.TransactionType {

		t.Fatal(
			"CONNECT transaction type metadata missing",
		)
	}
}

var _ paymentIntentClient = (*recordingPaymentIntentClient)(nil)

var _ refundClient = (*recordingRefundClient)(nil)
