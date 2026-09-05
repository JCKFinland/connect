package paymentcallback

import (
	"context"
	"net/http"
)

type VerifiedCallback struct {
	Provider string

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
