package payouts

import (
	"errors"
	"testing"
)

func fullyReadyProvider() ProviderReadiness {
	return ProviderReadiness{
		AuthoritativeLookupVerified: true,
		ReadOnlyTransportVerified:   true,
		SubmissionIdempotencyTested: true,
		DestinationBindingTested:    true,
		TimeoutRecoveryTested:       true,
		ProviderCredentialsReviewed: true,
		OperationalApprovalGranted:  true,
	}
}

func TestProviderReadinessRejectsZeroValue(t *testing.T) {
	err := ValidateProviderReadiness(ProviderReadiness{})

	if !errors.Is(err, ErrProviderNotReady) {
		t.Fatalf("zero-value readiness accepted: %v", err)
	}
}

func TestProviderReadinessAcceptsCompleteAttestation(t *testing.T) {
	err := ValidateProviderReadiness(fullyReadyProvider())

	if err != nil {
		t.Fatalf("complete attestation rejected: %v", err)
	}
}

func TestProviderReadinessRejectsEachMissingRequirement(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*ProviderReadiness)
	}{
		{
			name: "authoritative lookup",
			remove: func(r *ProviderReadiness) {
				r.AuthoritativeLookupVerified = false
			},
		},
		{
			name: "read-only transport",
			remove: func(r *ProviderReadiness) {
				r.ReadOnlyTransportVerified = false
			},
		},
		{
			name: "submission idempotency",
			remove: func(r *ProviderReadiness) {
				r.SubmissionIdempotencyTested = false
			},
		},
		{
			name: "destination binding",
			remove: func(r *ProviderReadiness) {
				r.DestinationBindingTested = false
			},
		},
		{
			name: "timeout recovery",
			remove: func(r *ProviderReadiness) {
				r.TimeoutRecoveryTested = false
			},
		},
		{
			name: "credential review",
			remove: func(r *ProviderReadiness) {
				r.ProviderCredentialsReviewed = false
			},
		},
		{
			name: "operational approval",
			remove: func(r *ProviderReadiness) {
				r.OperationalApprovalGranted = false
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readiness := fullyReadyProvider()
			tt.remove(&readiness)

			err := ValidateProviderReadiness(readiness)

			if !errors.Is(err, ErrProviderNotReady) {
				t.Fatalf(
					"missing %s requirement accepted: %v",
					tt.name,
					err,
				)
			}
		})
	}
}
