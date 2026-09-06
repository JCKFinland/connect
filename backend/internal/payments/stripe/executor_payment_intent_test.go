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
