# Mangopay G1: Immutable-Key Recovery Requirements

Status: BLOCKED
Provider approval: NOT GRANTED
Financial activation: DISABLED

## 1. CONNECT identity contract

CONNECT generates an immutable payout execution identity
and a stable idempotency key before provider submission.

The original key must identify the same transfer throughout
submission, reconciliation and recovery.

## 2. Lost-response scenario

Consider the following sequence:

1. CONNECT authorizes one provider initiation.
2. The provider may accept the payout.
3. The network connection fails before CONNECT receives
   the provider payout identifier.
4. CONNECT records an uncertain outcome.
5. Recovery must determine whether the original transfer
   exists without initiating another payout.

The recovery path must be read-only.

## 3. Required provider capability

A viable provider integration must establish an authoritative
way to resolve the original transfer using the immutable
CONNECT idempotency key or a separately approved,
independently verifiable provider-side binding.

A lookup requiring only a provider payout ID is insufficient
when that ID was lost with the initiation response.

The adapter must not invent a provider reference or infer
success from a locally recorded submission attempt.

## 4. Identity evidence

An independently retrieved provider record must establish:

- Original CONNECT payout identity or verified binding.
- Original immutable idempotency key or verified binding.
- Actual recipient destination.
- Actual transferred amount.
- Currency.
- Authoritative transfer status.
- Stable provider payout reference.

CONNECT must reject incomplete or mismatched evidence.

## 5. Idempotency requirements

Provider-specific documentation and tests must establish:

- Whether initiation accepts a caller-supplied idempotency key.
- Whether duplicate requests are deduplicated.
- How long deduplication remains effective.
- Whether an existing transfer can be queried by that key.
- Whether duplicate responses preserve the original payout ID.
- What happens after an uncertain network timeout.

Provider idempotency alone does not authorize CONNECT to
retry an uncertain submission.

## 6. Required failure tests

Before approving the integration, test:

1. Timeout before provider acceptance.
2. Timeout after provider acceptance.
3. Provider payout ID missing from the response.
4. Read-only lookup unavailable.
5. Provider reports pending status.
6. Provider reports failed status.
7. Provider reports success with mismatched destination.
8. Provider reports success with mismatched amount.
9. Duplicate initiation attempt.
10. Reconciliation after provider lookup errors.

No test may move real money.

## 7. Acceptance criteria

G1 can be marked resolved only when:

- Authoritative provider documentation identifies a safe
  recovery mechanism.
- Provider-specific tests demonstrate lost-response recovery.
- The lookup does not initiate a transfer.
- The retrieved evidence satisfies CONNECT's verification
  contract without fabricated fields.
- Timeout handling cannot create duplicate payouts.
- The engineering and security review approves the mapping.

## 8. Current decision

Mangopay remains a candidate.

G1 is BLOCKED until the provider-specific recovery
mechanism is independently established.

No production adapter or financial activation is authorized.
