package paymentcallback

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type fakeCallbackVerifier struct{}

func (fakeCallbackVerifier) Verify(
	context.Context,
	http.Header,
	[]byte,
) (*VerifiedCallback, error) {
	return &VerifiedCallback{
		Provider: "TEST_PROVIDER",

		ProviderTransactionID: "provider-123",

		ProviderStatus: "SUCCESS",
	}, nil
}

func TestVerifierRegistryRegisterAndGet(
	t *testing.T,
) {
	registry := NewVerifierRegistry()

	err := registry.Register(
		"test_provider",
		fakeCallbackVerifier{},
	)
	if err != nil {
		t.Fatalf(
			"register verifier: %v",
			err,
		)
	}

	verifier, err :=
		registry.Get(
			"TEST_PROVIDER",
		)
	if err != nil {
		t.Fatalf(
			"get verifier: %v",
			err,
		)
	}

	if verifier == nil {
		t.Fatal(
			"expected verifier",
		)
	}
}

func TestVerifierRegistryRejectsUnknownProvider(
	t *testing.T,
) {
	registry := NewVerifierRegistry()

	_, err :=
		registry.Get(
			"UNKNOWN_PROVIDER",
		)

	if !errors.Is(
		err,
		ErrUnsupportedCallbackProvider,
	) {
		t.Fatalf(
			"expected ErrUnsupportedCallbackProvider, got %v",
			err,
		)
	}
}

func TestVerifierRegistryRejectsDuplicateProvider(
	t *testing.T,
) {
	registry := NewVerifierRegistry()

	if err := registry.Register(
		"TEST_PROVIDER",
		fakeCallbackVerifier{},
	); err != nil {
		t.Fatalf(
			"register first verifier: %v",
			err,
		)
	}

	err := registry.Register(
		"test_provider",
		fakeCallbackVerifier{},
	)

	if !errors.Is(
		err,
		ErrCallbackVerifierAlreadyRegistered,
	) {
		t.Fatalf(
			"expected ErrCallbackVerifierAlreadyRegistered, got %v",
			err,
		)
	}
}
