package driver_payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseFencedVerificationDenied = errors.New(
	"lease-fenced payout verification denied",
)

// LeaseFencedVerificationService verifies existing transfers using
// authoritative database identity and independent provider evidence.
//
// It never initiates transfers, changes financial records,
// or finalizes payouts.
type LeaseFencedVerificationService struct {
	db       *pgxpool.Pool
	verifier *VerifiedReconciliationService
}

func NewLeaseFencedVerificationService(
	db *pgxpool.Pool,
	provider payouts.TransferDetailsProvider,
) (*LeaseFencedVerificationService, error) {
	if db == nil {
		return nil, ErrLeaseFencedVerificationDenied
	}

	verifier, err := NewVerifiedReconciliationService(provider)
	if err != nil {
		return nil, err
	}

	return &LeaseFencedVerificationService{
		db:       db,
		verifier: verifier,
	}, nil
}

// VerifyExistingTransfer requires an independently acquired
// recovery lease. It does not acquire or release the lease.
//
// requiredRemaining must be positive and no greater than five minutes.
// The caller is responsible for choosing a margin sufficient for
// its provider lookup timeout.
func (s *LeaseFencedVerificationService) VerifyExistingTransfer(
	ctx context.Context,
	payoutID string,
	leaseToken string,
	requiredRemaining time.Duration,
) (*payouts.VerifiedTransferDetails, error) {
	if s == nil || s.db == nil || s.verifier == nil ||
		ctx == nil || ctx.Err() != nil ||
		requiredRemaining <= 0 ||
		requiredRemaining > 5*time.Minute {
		return nil, ErrLeaseFencedVerificationDenied
	}

	// Fence the provider lookup before reading the transfer request.
	valid, err := postgresrepo.ValidateDriverPayoutRecoveryLease(
		ctx,
		s.db,
		payoutID,
		leaseToken,
		requiredRemaining,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: lease validation: %v",
			ErrLeaseFencedVerificationDenied,
			err,
		)
	}
	if !valid {
		return nil, ErrLeaseFencedVerificationDenied
	}

	expected, err :=
		postgresrepo.ResolveDriverPayoutRecoveryTransferRequest(
			ctx, s.db, payoutID,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: authoritative identity: %v",
			ErrLeaseFencedVerificationDenied,
			err,
		)
	}

	// Revalidate after the database read and before the external call.
	valid, err = postgresrepo.ValidateDriverPayoutRecoveryLease(
		ctx,
		s.db,
		payoutID,
		leaseToken,
		requiredRemaining,
	)
	if err != nil || !valid {
		return nil, ErrLeaseFencedVerificationDenied
	}

	actual, err := s.verifier.VerifyExistingTransferDetails(
		ctx, *expected,
	)
	if err != nil {
		return nil, err
	}

	// A lease may expire or be superseded during the provider lookup.
	// Never return verified evidence under a stale fencing token.
	valid, err = postgresrepo.ValidateDriverPayoutRecoveryLease(
		ctx,
		s.db,
		payoutID,
		leaseToken,
		requiredRemaining,
	)
	if err != nil || !valid {
		return nil, ErrLeaseFencedVerificationDenied
	}

	return actual, nil
}
