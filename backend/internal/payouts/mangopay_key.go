package payouts

import (
	"errors"
	"regexp"
	"strings"
)

var ErrInvalidMangopayKey = errors.New(
	"invalid Mangopay payout idempotency identity",
)

const mangopayInternalKeyPrefix = "driver-payout-execution:"

var mangopayUUIDPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

// DeriveMangopayIdempotencyKey derives a provider-compatible key
// from CONNECT's immutable internal payout execution identity.
//
// It performs no network, database, or financial operations.
func DeriveMangopayIdempotencyKey(
	internalKey string,
) (string, error) {
	if !strings.HasPrefix(
		internalKey,
		mangopayInternalKeyPrefix,
	) {
		return "", ErrInvalidMangopayKey
	}

	payoutID := strings.TrimPrefix(
		internalKey,
		mangopayInternalKeyPrefix,
	)

	if !mangopayUUIDPattern.MatchString(payoutID) {
		return "", ErrInvalidMangopayKey
	}

	return payoutID, nil
}
