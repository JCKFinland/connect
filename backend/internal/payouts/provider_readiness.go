package payouts

import "errors"

// ErrProviderNotReady indicates that a payout provider has not
// satisfied CONNECT's production integration requirements.
var ErrProviderNotReady = errors.New(
	"payout provider not ready for production",
)

// ProviderReadiness records independently established evidence
// required before a payout provider can be considered for activation.
//
// These flags are attestations, not runtime verification. They must
// only be set after provider-specific implementation, security review,
// integration testing, and operational approval.
//
// This structure does not activate a provider or authorize transfers.
type ProviderReadiness struct {
	AuthoritativeLookupVerified bool
	ReadOnlyTransportVerified   bool
	SubmissionIdempotencyTested bool
	DestinationBindingTested    bool
	TimeoutRecoveryTested       bool
	ProviderCredentialsReviewed bool
	OperationalApprovalGranted  bool
}

// ValidateProviderReadiness rejects incomplete provider integration
// readiness. It does not inspect provider credentials, contact a
// provider, start workers, or perform any financial operations.
func ValidateProviderReadiness(
	readiness ProviderReadiness,
) error {
	if !readiness.AuthoritativeLookupVerified ||
		!readiness.ReadOnlyTransportVerified ||
		!readiness.SubmissionIdempotencyTested ||
		!readiness.DestinationBindingTested ||
		!readiness.TimeoutRecoveryTested ||
		!readiness.ProviderCredentialsReviewed ||
		!readiness.OperationalApprovalGranted {
		return ErrProviderNotReady
	}

	return nil
}
