# Mangopay Driver Payout Compatibility Assessment

Status: NOT APPROVED
Scope: Public API documentation review only
Financial activation: DISABLED

## Confirmed documented capabilities

- Bank-wire payout creation endpoint.
- EUR transfer amounts expressed in minor units.
- RecipientId destination reference.
- Payout retrieval using Mangopay payout ID.
- Payout status, currency, amounts and recipient fields.
- Standard SEPA and SEPA Instant payout modes.

These are documentation findings, not integration-test results.

## Critical compatibility gaps

### G1: Immutable-key lookup

CONNECT requires read-only authoritative lookup using its
immutable idempotency key.

The reviewed Mangopay payout retrieval endpoint uses
a Mangopay payout ID.

No complete, independently verified key-to-payout lookup
design has been established.

Status: BLOCKED

### G2: Authoritative identity evidence

CONNECT requires independently retrieved evidence for:

- PayoutID
- IdempotencyKey
- DestinationID
- AmountCents
- Currency
- ProviderReference
- SUCCEEDED status

Mangopay documents Tag, RecipientId, payout ID,
amounts, currency and status.

A complete provider-side authoritative mapping for all
CONNECT identity fields has not been established.

Fields must never be reconstructed from CONNECT's
expected transfer request and presented as provider evidence.

Status: BLOCKED

### G3: Timeout and duplicate prevention

Provider-specific idempotency behavior and recovery after
an uncertain payout submission must be established.

A timeout must not authorize automatic resubmission.

Status: BLOCKED

### G4: Retention

Mangopay documents a 13-month payout retrieval window.

CONNECT requires a separately reviewed archival,
reconciliation and audit-retention policy.

Status: OPEN

### G5: Commercial and operational eligibility

Finnish business eligibility, contracts, wallet funding,
recipient onboarding, pricing, credentials and compliance
requirements have not been verified.

Status: OPEN

## Source documentation

Create a Payout:
https://docs.mangopay.com/api-reference/payouts/create-payout

View a Payout:
https://docs.mangopay.com/api-reference/payouts/view-payout

View a Payout and check mode applied:
https://docs.mangopay.com/api-reference/payouts/view-payout-check-mode-applied

The Payout object:
https://docs.mangopay.com/api-reference/payouts/payout-object

## Engineering decision

Mangopay is a research candidate only.

Do not implement or activate a production adapter until
G1, G2 and G3 are resolved with authoritative provider
documentation and provider-specific tests.

Do not initiate transfers or enable recovery finalization.
