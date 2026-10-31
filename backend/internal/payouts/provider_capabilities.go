package payouts

import (
	"errors"
	"reflect"
	"strings"
)

// ErrInvalidProviderCapabilities indicates that a payout provider
// is not eligible for independently verified financial recovery.
var ErrInvalidProviderCapabilities = errors.New(
	"invalid payout provider capabilities",
)

// ProviderCapabilities describes the two independent capabilities
// required by CONNECT's driver payout execution architecture.
//
// Submission may initiate a transfer at most once.
// Verification must only retrieve existing provider-side records.
//
// This structure does not initiate transfers or perform lookups.
type ProviderCapabilities struct {
	Name       string
	Submission Provider
	Details    TransferDetailsProvider
}

// ValidateProviderCapabilities verifies that both required
// capabilities are configured and that the provider has an
// explicit, stable identity.
//
// This validation does not prove that a provider implementation
// is operationally safe. Provider-specific contract tests must
// independently establish read-only lookup behavior and
// authoritative transfer identity before production activation.
func ValidateProviderCapabilities(
	capabilities ProviderCapabilities,
) error {
	name := capabilities.Name

	if strings.TrimSpace(name) == "" ||
		name != strings.TrimSpace(name) ||
		len(name) > 100 ||
		isNilProviderCapability(capabilities.Submission) ||
		isNilProviderCapability(capabilities.Details) {
		return ErrInvalidProviderCapabilities
	}

	return nil
}

// isNilProviderCapability rejects typed-nil interface values
// without invoking any provider method.
func isNilProviderCapability(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
