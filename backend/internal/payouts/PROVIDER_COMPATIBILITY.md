# CONNECT Driver Payout Provider Compatibility

## Status

Specification only. No provider has been approved or activated.

This document describes minimum requirements for a prospective
provider supporting CONNECT driver bank-transfer payouts.

## 1. Transfer submission

A provider adapter must:

- Accept an immutable payout identity.
- Accept a stable idempotency key.
- Accept a previously verified destination identifier.
- Accept an exact amount in integer cents.
- Support EUR transfers.
- Never retry a transfer after an uncertain outcome without
  independent reconciliation.
- Preserve the same idempotency key throughout the transfer lifecycle.

CONNECT's submission service must authorize at most one
provider initiation per claimed payout execution.

## 2. Independent authoritative verification

The provider must support a read-only lookup of an existing
transfer using an immutable identifier.

The resulting authoritative evidence must establish:

- Transfer success status.
- CONNECT payout identity, or an independently verifiable
  provider-side binding to that identity.
- Original idempotency key.
- Actual destination identifier.
- Actual transferred amount.
- Currency.
- Stable provider transfer reference.

The adapter must not fabricate missing fields using CONNECT's
expected request.

A provider that exposes only a status is insufficient.

If provider records do not directly contain all required fields,
a separately reviewed and tested authoritative identity-mapping
design is required before integration.

## 3. Destination requirements

CONNECT binds each execution to a verified destination belonging
to the correct driver and company.

Before approval, the provider integration must establish:

- How bank accounts or beneficiary destinations are registered.
- How destination ownership and verification are performed.
- Whether IBAN and BIC are required or supported.
- Whether SEPA Credit Transfer is supported.
- Whether SEPA Instant Credit Transfer is supported, if needed.
- How destination changes, revocations, and failed verification
  are represented.

CONNECT must not treat an unverified destination as eligible
for financial execution.

## 4. Failure and recovery

The provider integration must support investigation of:

- Request timeout after possible acceptance.
- Network interruption after possible acceptance.
- Provider-reported pending transfers.
- Provider-reported failures.
- Delayed successful settlement.
- Duplicate idempotency-key submission attempts.
- Unavailable authoritative lookup.
- Provider identity or amount mismatch.

Unknown outcomes must remain unresolved until independently
verified. They must not trigger automatic resubmission.

## 5. Transport and security

Before approval, the provider-specific adapter must demonstrate:

- Secure authenticated provider communication.
- Verified TLS configuration and endpoint identity.
- Strict response parsing and bounded response sizes.
- Redirect handling that prevents unintended requests.
- Timeouts and cancellation propagation.
- Credential storage and rotation procedures.
- Protection of sensitive destination and transfer information.
- Safe audit logging without exposing credentials.

Synthetic HTTP transport tests alone do not establish
production transport security.

## 6. Financial authorization

Provider-reported success is not sufficient to finalize
CONNECT's financial records.

Finalization requires independently verified transfer evidence,
a valid recovery lease, and the existing database consistency
checks.

Financial finalization remains disabled by default.

## 7. Production readiness

The provider must pass:

1. Provider-specific authoritative lookup tests.
2. Read-only transport verification.
3. Submission idempotency testing.
4. Destination binding verification.
5. Timeout and unknown-outcome recovery testing.
6. Credential and security review.
7. Explicit operational approval.

The ProviderReadiness structure records attestations only.
It is not proof of compliance or permission to initiate transfers.

## 8. Outstanding decisions

The following remain unresolved:

- Selection of a licensed payout provider.
- Provider availability for Finnish businesses.
- Beneficiary onboarding and account-verification requirements.
- Actual SEPA and instant-payout capabilities.
- Fees, settlement times, and operational limits.
- Provider-side transfer identity and lookup API semantics.
- Applicable contractual, regulatory, and compliance obligations.

No provider adapter may be activated solely on the basis of
this specification.
