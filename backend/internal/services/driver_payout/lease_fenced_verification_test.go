package driver_payout

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLeaseFencedVerificationRejectsNilDatabase(t *testing.T) {
	provider := &verifiedReconciliationProvider{}

	service, err := NewLeaseFencedVerificationService(
		nil, provider,
	)
	if service != nil ||
		!errors.Is(err, ErrLeaseFencedVerificationDenied) {
		t.Fatalf(
			"nil database accepted: service=%+v err=%v",
			service, err,
		)
	}
}

func TestLeaseFencedVerificationRejectsNilService(t *testing.T) {
	var service *LeaseFencedVerificationService

	result, err := service.VerifyExistingTransfer(
		context.Background(),
		"invalid-payout",
		"invalid-token",
		time.Second,
	)

	if result != nil ||
		!errors.Is(err, ErrLeaseFencedVerificationDenied) {
		t.Fatalf(
			"nil service accepted: result=%+v err=%v",
			result, err,
		)
	}
}

func TestLeaseFencedVerificationRejectsInvalidDuration(t *testing.T) {
	service := &LeaseFencedVerificationService{}

	for _, duration := range []time.Duration{
		0,
		-time.Second,
		6 * time.Minute,
	} {
		result, err := service.VerifyExistingTransfer(
			context.Background(),
			"invalid-payout",
			"invalid-token",
			duration,
		)

		if result != nil ||
			!errors.Is(err, ErrLeaseFencedVerificationDenied) {
			t.Fatalf(
				"invalid duration %v accepted: result=%+v err=%v",
				duration, result, err,
			)
		}
	}
}
