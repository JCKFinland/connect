package paymentcallback

import (
	"context"
	"net/http"
)

type VerifiedCallback struct {
	Provider string

	// TransactionID identifies the exact CONNECT financial operation
	// referenced by the verified provider callback.
	//
	// Providers that support trusted metadata should populate this value so
	// callbacks do not depend on provider resource IDs being globally unique
	// across CONNECT operations.
	TransactionID string

	// PaymentID identifies the CONNECT aggregate payment referenced by the
	// verified provider callback.
	PaymentID string

	// ProviderTransactionID identifies the provider-side payment resource.
	//
	// A provider resource may legitimately be shared by multiple CONNECT
	// operations. For example, Stripe uses the same PaymentIntent for
	// AUTHORIZE and CAPTURE.
	ProviderTransactionID string

	ProviderStatus string

	RawPayload []byte
}

type Verifier interface {
	Verify(
		ctx context.Context,
		headers http.Header,
		rawBody []byte,
	) (*VerifiedCallback, error)
}
