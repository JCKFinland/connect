package paymentcallback

import (
	"fmt"
	"strings"
)

type VerifierRegistry struct {
	verifiers map[string]Verifier
}

func NewVerifierRegistry() *VerifierRegistry {
	return &VerifierRegistry{
		verifiers: make(
			map[string]Verifier,
		),
	}
}

func (r *VerifierRegistry) Register(
	provider string,
	verifier Verifier,
) error {
	name := strings.TrimSpace(
		strings.ToUpper(provider),
	)

	if name == "" {
		return fmt.Errorf(
			"provider is required",
		)
	}

	if verifier == nil {
		return fmt.Errorf(
			"verifier is required",
		)
	}

	if _, exists := r.verifiers[name]; exists {
		return fmt.Errorf(
			"%w: %s",
			ErrCallbackVerifierAlreadyRegistered,
			name,
		)
	}

	r.verifiers[name] = verifier

	return nil
}

func (r *VerifierRegistry) Get(
	provider string,
) (Verifier, error) {
	name := strings.TrimSpace(
		strings.ToUpper(provider),
	)

	verifier, ok := r.verifiers[name]
	if !ok {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedCallbackProvider,
			provider,
		)
	}

	return verifier, nil
}
